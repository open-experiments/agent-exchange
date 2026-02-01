"""A2A Protocol server implementation."""

import asyncio
import json
import logging
import uuid
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from enum import Enum
from typing import Any, AsyncIterator, Callable, Optional
from aiohttp import web

from .agent_card import AgentCard

logger = logging.getLogger(__name__)


class TaskState(str, Enum):
    """A2A Task states."""
    SUBMITTED = "submitted"
    WORKING = "working"
    INPUT_REQUIRED = "input-required"
    COMPLETED = "completed"
    CANCELED = "canceled"
    FAILED = "failed"


@dataclass
class Message:
    """A2A Message."""
    role: str  # "user" or "agent"
    parts: list[dict]
    messageId: str = field(default_factory=lambda: str(uuid.uuid4()))

    @classmethod
    def text(cls, role: str, text: str) -> "Message":
        """Create a text message."""
        return cls(role=role, parts=[{"type": "text", "text": text}])


@dataclass
class Task:
    """A2A Task."""
    id: str
    sessionId: str
    status: dict
    history: list[Message] = field(default_factory=list)
    artifacts: list[dict] = field(default_factory=list)
    metadata: dict = field(default_factory=dict)

    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "id": self.id,
            "sessionId": self.sessionId,
            "status": self.status,
            "history": [
                {
                    "role": m.role,
                    "parts": m.parts,
                    "messageId": m.messageId,
                }
                for m in self.history
            ],
            "artifacts": self.artifacts,
            "metadata": self.metadata,
        }


class A2AHandler(ABC):
    """Abstract handler for A2A requests."""

    @abstractmethod
    async def handle_message(
        self,
        task_id: str,
        session_id: str,
        message: Message,
        context: dict,
    ) -> AsyncIterator[dict]:
        """
        Handle an incoming message and yield response events.

        Yields status updates and final result:
        - {"type": "status", "state": "working", "message": "Processing..."}
        - {"type": "artifact", "name": "result.txt", "parts": [...]}
        - {"type": "result", "parts": [...]}
        """
        yield {"type": "status", "state": "working"}

    async def validate_token(self, token: str) -> Optional[dict]:
        """
        Validate contract token from AEX.
        Returns token claims if valid, None if invalid.
        Override this method to implement token validation.
        """
        # For demo, accept all tokens
        return {"contract_id": "demo", "valid": True}


class A2AServer:
    """A2A Protocol compliant server."""

    def __init__(
        self,
        agent_card: AgentCard,
        handler: A2AHandler,
        require_auth: bool = False,
    ):
        self.agent_card = agent_card
        self.handler = handler
        self.require_auth = require_auth
        self.tasks: dict[str, Task] = {}
        self.app = web.Application()
        self._setup_routes()

    def _setup_routes(self):
        """Setup HTTP routes."""
        self.app.router.add_get("/.well-known/agent-card.json", self._handle_agent_card)
        self.app.router.add_post("/a2a", self._handle_jsonrpc)
        self.app.router.add_get("/health", self._handle_health)

    async def _handle_health(self, request: web.Request) -> web.Response:
        """Health check endpoint."""
        return web.json_response({"status": "healthy", "agent": self.agent_card.name})

    async def _handle_agent_card(self, request: web.Request) -> web.Response:
        """Serve the agent card."""
        return web.json_response(self.agent_card.to_dict())

    async def _handle_jsonrpc(self, request: web.Request) -> web.Response:
        """Handle A2A JSON-RPC requests."""
        try:
            body = await request.json()
        except json.JSONDecodeError:
            return self._error_response(None, -32700, "Parse error")

        # Validate JSON-RPC structure
        if body.get("jsonrpc") != "2.0":
            return self._error_response(body.get("id"), -32600, "Invalid Request")

        method = body.get("method")
        params = body.get("params", {})
        request_id = body.get("id")

        # Validate auth if required
        if self.require_auth:
            auth_header = request.headers.get("Authorization", "")
            if not auth_header.startswith("Bearer "):
                return self._error_response(request_id, -32001, "Unauthorized")
            token = auth_header[7:]
            claims = await self.handler.validate_token(token)
            if not claims:
                return self._error_response(request_id, -32001, "Invalid token")
            params["_auth"] = claims

        # Route to handler
        handlers = {
            "message/send": self._handle_message_send,
            "message/stream": self._handle_message_stream,
            "tasks/get": self._handle_tasks_get,
            "tasks/cancel": self._handle_tasks_cancel,
        }

        handler_fn = handlers.get(method)
        if not handler_fn:
            return self._error_response(request_id, -32601, f"Method not found: {method}")

        try:
            result = await handler_fn(params)
            return web.json_response({
                "jsonrpc": "2.0",
                "id": request_id,
                "result": result,
            })
        except Exception as e:
            logger.exception(f"Error handling {method}")
            return self._error_response(request_id, -32603, str(e))

    async def _handle_message_send(self, params: dict) -> dict:
        """Handle message/send - synchronous message handling."""
        message_data = params.get("message", {})
        session_id = params.get("sessionId", str(uuid.uuid4()))

        # Create message
        message = Message(
            role=message_data.get("role", "user"),
            parts=message_data.get("parts", []),
            messageId=message_data.get("messageId", str(uuid.uuid4())),
        )

        # Create or get task
        task_id = str(uuid.uuid4())
        task = Task(
            id=task_id,
            sessionId=session_id,
            status={"state": TaskState.SUBMITTED.value},
            history=[message],
        )
        self.tasks[task_id] = task

        # Process message
        context = params.get("_auth", {})
        response_parts = []
        artifacts = []

        async for event in self.handler.handle_message(task_id, session_id, message, context):
            if event["type"] == "status":
                task.status = {"state": event["state"], "message": event.get("message")}
            elif event["type"] == "artifact":
                artifacts.append({
                    "name": event.get("name", "result"),
                    "parts": event.get("parts", []),
                })
            elif event["type"] == "result":
                response_parts = event.get("parts", [])

        # Update task
        task.status = {"state": TaskState.COMPLETED.value}
        task.artifacts = artifacts

        # Add agent response to history
        if response_parts:
            agent_message = Message(role="agent", parts=response_parts)
            task.history.append(agent_message)

        return task.to_dict()

    async def _handle_message_stream(self, params: dict) -> dict:
        """Handle message/stream - streaming message handling."""
        # For simplicity, delegate to send and return task reference
        # Real streaming would use SSE
        result = await self._handle_message_send(params)
        return result

    async def _handle_tasks_get(self, params: dict) -> dict:
        """Get task status."""
        task_id = params.get("id")
        if not task_id or task_id not in self.tasks:
            raise ValueError(f"Task not found: {task_id}")
        return self.tasks[task_id].to_dict()

    async def _handle_tasks_cancel(self, params: dict) -> dict:
        """Cancel a task."""
        task_id = params.get("id")
        if not task_id or task_id not in self.tasks:
            raise ValueError(f"Task not found: {task_id}")
        task = self.tasks[task_id]
        task.status = {"state": TaskState.CANCELED.value}
        return task.to_dict()

    def _error_response(self, request_id: Any, code: int, message: str) -> web.Response:
        """Create JSON-RPC error response."""
        return web.json_response({
            "jsonrpc": "2.0",
            "id": request_id,
            "error": {"code": code, "message": message},
        })

    async def run_async(self, host: str = "0.0.0.0", port: int = 8100):
        """Run the server asynchronously."""
        logger.info(f"Starting A2A server at http://{host}:{port}")
        logger.info(f"Agent Card: http://{host}:{port}/.well-known/agent-card.json")
        runner = web.AppRunner(self.app)
        await runner.setup()
        site = web.TCPSite(runner, host, port)
        await site.start()
        # Keep running until interrupted
        try:
            while True:
                await asyncio.sleep(3600)
        except asyncio.CancelledError:
            await runner.cleanup()

    def run(self, host: str = "0.0.0.0", port: int = 8100):
        """Run the server synchronously."""
        logger.info(f"Starting A2A server at http://{host}:{port}")
        logger.info(f"Agent Card: http://{host}:{port}/.well-known/agent-card.json")
        web.run_app(self.app, host=host, port=port)

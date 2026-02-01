"""Client for connecting to OpenClaw/Molt.bot Gateway via WebSocket."""

import asyncio
import json
import logging
from dataclasses import dataclass, field
from typing import Any, Callable, Optional
from uuid import uuid4

import aiohttp

logger = logging.getLogger(__name__)


@dataclass
class OpenClawMessage:
    """Message format for OpenClaw gateway."""
    id: str
    method: str
    params: dict

    def to_json(self) -> str:
        return json.dumps({
            "jsonrpc": "2.0",
            "id": self.id,
            "method": self.method,
            "params": self.params,
        })

    @classmethod
    def from_json(cls, data: str) -> "OpenClawMessage":
        parsed = json.loads(data)
        return cls(
            id=parsed.get("id", ""),
            method=parsed.get("method", ""),
            params=parsed.get("params", {}),
        )


@dataclass
class OpenClawClient:
    """WebSocket client for OpenClaw/Molt.bot Gateway."""

    gateway_url: str = "ws://localhost:18789"
    agent_id: str = ""
    agent_name: str = ""

    _ws: Optional[aiohttp.ClientWebSocketResponse] = field(default=None, init=False)
    _session: Optional[aiohttp.ClientSession] = field(default=None, init=False)
    _connected: bool = field(default=False, init=False)
    _message_handlers: dict[str, Callable] = field(default_factory=dict, init=False)
    _pending_requests: dict[str, asyncio.Future] = field(default_factory=dict, init=False)
    _receive_task: Optional[asyncio.Task] = field(default=None, init=False)

    async def connect(self) -> bool:
        """Connect to the OpenClaw gateway."""
        try:
            self._session = aiohttp.ClientSession()
            self._ws = await self._session.ws_connect(
                self.gateway_url,
                heartbeat=30,
            )
            self._connected = True

            # Start message receiver
            self._receive_task = asyncio.create_task(self._receive_loop())

            # Register this agent with the gateway
            await self._register_agent()

            logger.info(f"Connected to OpenClaw gateway: {self.gateway_url}")
            return True

        except Exception as e:
            logger.error(f"Failed to connect to OpenClaw gateway: {e}")
            self._connected = False
            return False

    async def disconnect(self):
        """Disconnect from the gateway."""
        self._connected = False

        if self._receive_task:
            self._receive_task.cancel()
            try:
                await self._receive_task
            except asyncio.CancelledError:
                pass

        if self._ws and not self._ws.closed:
            await self._ws.close()

        if self._session and not self._session.closed:
            await self._session.close()

        logger.info("Disconnected from OpenClaw gateway")

    async def _register_agent(self):
        """Register this agent with the gateway."""
        response = await self.send_request("agent.register", {
            "agent_id": self.agent_id,
            "name": self.agent_name,
            "capabilities": ["a2a", "token_payments"],
            "version": "1.0.0",
        })
        logger.info(f"Agent registered: {response}")

    async def _receive_loop(self):
        """Background task to receive messages from gateway."""
        try:
            async for msg in self._ws:
                if msg.type == aiohttp.WSMsgType.TEXT:
                    await self._handle_message(msg.data)
                elif msg.type == aiohttp.WSMsgType.ERROR:
                    logger.error(f"WebSocket error: {self._ws.exception()}")
                    break
                elif msg.type == aiohttp.WSMsgType.CLOSED:
                    logger.info("WebSocket closed")
                    break
        except asyncio.CancelledError:
            pass
        except Exception as e:
            logger.error(f"Receive loop error: {e}")
        finally:
            self._connected = False

    async def _handle_message(self, data: str):
        """Handle incoming message from gateway."""
        try:
            parsed = json.loads(data)

            # Check if this is a response to a pending request
            msg_id = parsed.get("id")
            if msg_id and msg_id in self._pending_requests:
                future = self._pending_requests.pop(msg_id)
                if "error" in parsed:
                    future.set_exception(Exception(parsed["error"]))
                else:
                    future.set_result(parsed.get("result"))
                return

            # Check if this is an incoming method call
            method = parsed.get("method", "")
            if method and method in self._message_handlers:
                handler = self._message_handlers[method]
                result = await handler(parsed.get("params", {}))

                # Send response if there's an id
                if msg_id:
                    await self._send_response(msg_id, result)

        except Exception as e:
            logger.error(f"Error handling message: {e}")

    async def _send_response(self, msg_id: str, result: Any):
        """Send response to a request."""
        response = {
            "jsonrpc": "2.0",
            "id": msg_id,
            "result": result,
        }
        await self._ws.send_str(json.dumps(response))

    async def send_request(self, method: str, params: dict, timeout: float = 30.0) -> Any:
        """Send a request and wait for response."""
        if not self._connected or not self._ws:
            raise ConnectionError("Not connected to gateway")

        msg_id = str(uuid4())
        message = {
            "jsonrpc": "2.0",
            "id": msg_id,
            "method": method,
            "params": params,
        }

        # Create future for response
        future = asyncio.get_event_loop().create_future()
        self._pending_requests[msg_id] = future

        try:
            await self._ws.send_str(json.dumps(message))
            return await asyncio.wait_for(future, timeout=timeout)
        except asyncio.TimeoutError:
            self._pending_requests.pop(msg_id, None)
            raise TimeoutError(f"Request {method} timed out")

    async def send_notification(self, method: str, params: dict):
        """Send a notification (no response expected)."""
        if not self._connected or not self._ws:
            raise ConnectionError("Not connected to gateway")

        message = {
            "jsonrpc": "2.0",
            "method": method,
            "params": params,
        }
        await self._ws.send_str(json.dumps(message))

    def on_message(self, method: str, handler: Callable):
        """Register a handler for incoming messages."""
        self._message_handlers[method] = handler

    # =========================================
    # Sessions API (agent-to-agent communication)
    # =========================================

    async def sessions_create(self, target_agent: str, initial_message: str = "") -> dict:
        """Create a new session with another agent."""
        return await self.send_request("sessions.create", {
            "target_agent": target_agent,
            "initial_message": initial_message,
            "from_agent": self.agent_id,
        })

    async def sessions_send(self, session_id: str, message: dict) -> dict:
        """Send a message to an existing session."""
        return await self.send_request("sessions.send", {
            "session_id": session_id,
            "message": message,
            "from_agent": self.agent_id,
        })

    async def sessions_list(self) -> list[dict]:
        """List all active sessions for this agent."""
        result = await self.send_request("sessions.list", {
            "agent_id": self.agent_id,
        })
        return result.get("sessions", [])

    async def sessions_close(self, session_id: str) -> bool:
        """Close a session."""
        result = await self.send_request("sessions.close", {
            "session_id": session_id,
            "agent_id": self.agent_id,
        })
        return result.get("success", False)

    # =========================================
    # Agent Discovery
    # =========================================

    async def discover_agents(self, capability: Optional[str] = None) -> list[dict]:
        """Discover other agents on the gateway."""
        params = {}
        if capability:
            params["capability"] = capability

        result = await self.send_request("agents.discover", params)
        return result.get("agents", [])

    async def get_agent_info(self, agent_id: str) -> Optional[dict]:
        """Get information about a specific agent."""
        try:
            return await self.send_request("agents.info", {"agent_id": agent_id})
        except Exception:
            return None

    # =========================================
    # Helper Methods
    # =========================================

    async def send_to_agent(
        self,
        target_agent: str,
        action: str,
        data: dict,
    ) -> dict:
        """
        High-level method to send a message to another agent.
        Creates a session, sends the message, and returns the response.
        """
        # Create session
        session = await self.sessions_create(target_agent)
        session_id = session.get("session_id")

        if not session_id:
            raise Exception(f"Failed to create session with {target_agent}")

        try:
            # Send message
            message = {
                "action": action,
                "from_agent": self.agent_id,
                **data,
            }
            response = await self.sessions_send(session_id, message)
            return response
        finally:
            # Close session
            await self.sessions_close(session_id)

    @property
    def is_connected(self) -> bool:
        return self._connected

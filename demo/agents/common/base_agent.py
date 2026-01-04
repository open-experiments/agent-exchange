"""Base agent implementation using LangGraph."""

import asyncio
import logging
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import Any, AsyncIterator, Optional, TypedDict

from langgraph.graph import StateGraph, END

from .a2a_server import A2AHandler, Message, TaskState
from .aex_client import AEXClient, BidResponse
from .config import AgentConfig

logger = logging.getLogger(__name__)


class AgentState(TypedDict):
    """State for the agent graph."""
    messages: list[dict]
    task_id: str
    session_id: str
    context: dict
    result: Optional[str]
    artifacts: list[dict]
    status: str


@dataclass
class BaseAgent(A2AHandler, ABC):
    """Base class for LangGraph-based agents."""

    config: AgentConfig
    aex_client: Optional[AEXClient] = None
    _graph: Optional[StateGraph] = None

    def __post_init__(self):
        """Initialize the agent."""
        import os
        self._setup_llm()
        self._build_graph()
        if self.config.aex.enabled:
            self.aex_client = AEXClient(
                gateway_url=self.config.aex.gateway_url,
                api_key=os.environ.get("AEX_API_KEY", "dev-api-key"),
            )

    @abstractmethod
    def _setup_llm(self):
        """Setup the LLM client. Override in subclass."""
        pass

    @abstractmethod
    def _build_graph(self):
        """Build the LangGraph workflow. Override in subclass."""
        pass

    @abstractmethod
    async def process(self, state: AgentState) -> AgentState:
        """Process a message through the agent. Override in subclass."""
        pass

    async def handle_message(
        self,
        task_id: str,
        session_id: str,
        message: Message,
        context: dict,
    ) -> AsyncIterator[dict]:
        """Handle incoming A2A message."""
        yield {"type": "status", "state": TaskState.WORKING.value, "message": "Processing request..."}

        # Extract text from message parts
        text_content = ""
        for part in message.parts:
            if part.get("type") == "text":
                text_content += part.get("text", "")

        # Build initial state
        state: AgentState = {
            "messages": [{"role": message.role, "content": text_content}],
            "task_id": task_id,
            "session_id": session_id,
            "context": context,
            "result": None,
            "artifacts": [],
            "status": "working",
        }

        try:
            # Process through the graph
            final_state = await self.process(state)

            # Yield artifacts
            for artifact in final_state.get("artifacts", []):
                yield {
                    "type": "artifact",
                    "name": artifact.get("name", "result"),
                    "parts": artifact.get("parts", []),
                }

            # Yield final result
            result_text = final_state.get("result", "Processing complete.")
            yield {
                "type": "result",
                "parts": [{"type": "text", "text": result_text}],
            }

        except Exception as e:
            logger.exception(f"Error processing message: {e}")
            yield {
                "type": "result",
                "parts": [{"type": "text", "text": f"Error: {str(e)}"}],
            }

    async def calculate_bid(self, work_id: str, requirements: dict, budget: dict) -> Optional[BidResponse]:
        """
        Calculate bid for work request.
        Override in subclass for custom bidding logic.
        """
        if not self.config.aex.auto_bid:
            return None

        # Default: use base rate with small random variation
        import random
        price = self.config.aex.base_rate * (0.9 + random.random() * 0.2)

        return BidResponse(
            work_id=work_id,
            price=round(price, 2),
            currency=self.config.aex.currency,
            confidence=0.85 + random.random() * 0.1,
            estimated_duration_ms=30000,
        )

    async def handle_bid_request(self, bid_request: dict) -> Optional[dict]:
        """Handle incoming bid request from AEX webhook."""
        work_id = bid_request.get("work_id")
        requirements = bid_request.get("requirements", {})
        budget = bid_request.get("budget", {})

        bid = await self.calculate_bid(work_id, requirements, budget)
        if bid and self.aex_client:
            return await self.aex_client.submit_bid(bid)
        return None

    async def register_with_aex(self, base_url: str):
        """Register this agent with AEX."""
        if not self.aex_client or not self.config.aex.auto_register:
            return

        try:
            # Register provider
            await self.aex_client.register_provider(
                name=self.config.name,
                description=self.config.description,
                endpoint=base_url,
                bid_webhook=f"{base_url}/webhook/bid",
                capabilities=[s.id for s in self.config.skills],
            )

            # Subscribe to categories based on skill tags
            categories = set()
            for skill in self.config.skills:
                for tag in skill.tags:
                    categories.add(tag)
                    categories.add(f"{tag}/*")

            if categories:
                await self.aex_client.subscribe_to_categories(
                    categories=list(categories),
                    webhook_url=f"{base_url}/webhook/work",
                )

            logger.info(f"Registered with AEX: {self.config.name}")

        except Exception as e:
            logger.warning(f"Failed to register with AEX: {e}")

    async def close(self):
        """Cleanup resources."""
        if self.aex_client:
            await self.aex_client.close()

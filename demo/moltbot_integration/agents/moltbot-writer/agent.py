"""Moltbot Writer Agent - Provides content writing services for tokens using Claude LLM."""

import json
import logging
import os
from typing import AsyncIterator

import sys
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..'))

from common.moltbot_agent import MoltbotAgent, AgentConfig
from common.llm_client import WriterLLM

logger = logging.getLogger(__name__)


class WriterAgent(MoltbotAgent):
    """Writer agent that provides content creation and copywriting using Claude LLM."""

    def get_capabilities(self) -> list[str]:
        return ["copywriting", "technical_docs", "summaries", "content_creation"]

    async def handle_request(self, request: dict) -> AsyncIterator[dict]:
        """Handle writing request with Claude LLM."""
        topic = request.get("topic", request.get("text", ""))
        doc_type = request.get("type", "article")  # article, report, summary, technical_doc
        length = request.get("length", "medium")  # short, medium, long

        if not topic:
            yield {
                "type": "result",
                "parts": [{
                    "type": "text",
                    "text": json.dumps({
                        "error": "No topic provided",
                        "service_price": self.config.service_price,
                    })
                }]
            }
            return

        # Verify LLM is available
        if not self.llm_client or not self.llm_client.is_available:
            yield {
                "type": "result",
                "parts": [{
                    "type": "text",
                    "text": json.dumps({
                        "error": "LLM not available. Set ANTHROPIC_API_KEY to enable.",
                        "service_price": self.config.service_price,
                    })
                }]
            }
            return

        yield {
            "type": "status",
            "state": "working",
            "message": f"Writing {doc_type} about: {topic[:50]}..."
        }

        yield {
            "type": "status",
            "state": "working",
            "message": "Generating content with Claude LLM..."
        }

        # Use specialized writer method
        if isinstance(self.llm_client, WriterLLM):
            result = await self.llm_client.write(topic, doc_type=doc_type, length=length)
            writing_result = {
                "topic": topic,
                "type": doc_type,
                "content": result.get("content", ""),
                "model": self.config.llm_model,
                "service_cost": self.config.service_price,
                "provider": {
                    "agent_id": self.config.agent_id,
                    "agent_name": self.config.agent_name,
                }
            }
        else:
            # Fallback to generic LLM
            response = await self.llm_client.generate(
                prompt=f"Write a {length} {doc_type} about: {topic}",
                system="You are an AI writing assistant. Create clear, engaging, well-structured content."
            )
            writing_result = {
                "topic": topic,
                "type": doc_type,
                "content": response,
                "model": self.config.llm_model,
                "service_cost": self.config.service_price,
                "provider": {
                    "agent_id": self.config.agent_id,
                    "agent_name": self.config.agent_name,
                }
            }

        yield {
            "type": "result",
            "parts": [{
                "type": "text",
                "text": json.dumps(writing_result, indent=2)
            }]
        }


def create_agent() -> WriterAgent:
    """Create and configure the writer agent."""
    config = AgentConfig(
        agent_id="moltbot-writer",
        agent_name="Moltbot Writer",
        description="AI writing agent providing copywriting, technical documentation, and content creation services",
        port=int(os.environ.get("PORT", "8096")),
        initial_tokens=float(os.environ.get("INITIAL_TOKENS", "75")),
        service_price=float(os.environ.get("SERVICE_PRICE", "15")),
        token_bank_url=os.environ.get("TOKEN_BANK_URL", "http://aex-token-bank:8094"),
        aex_registry_url=os.environ.get("AEX_REGISTRY_URL", "http://aex-provider-registry:8080"),
        openclaw_gateway_url=os.environ.get("OPENCLAW_GATEWAY_URL", "ws://localhost:18789"),
        enable_gateway=os.environ.get("ENABLE_GATEWAY", "true").lower() == "true",
        enable_ap2=os.environ.get("ENABLE_AP2", "true").lower() == "true",
        bank_token=os.environ.get("BANK_TOKEN", ""),
        # Moltbook integration
        enable_moltbook=os.environ.get("ENABLE_MOLTBOOK", "true").lower() == "true",
        moltbook_api_key=os.environ.get("MOLTBOOK_API_KEY", ""),
        moltbook_base_url=os.environ.get("MOLTBOOK_BASE_URL", ""),
        # LLM integration
        enable_llm=True,  # Always enabled - required
        anthropic_api_key=os.environ.get("ANTHROPIC_API_KEY", ""),
        llm_model=os.environ.get("LLM_MODEL", "claude-sonnet-4-20250514"),
        agent_type="writer",
    )
    return WriterAgent(config=config)

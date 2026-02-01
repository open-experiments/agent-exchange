"""Moltbot Researcher Agent - Provides research services for tokens using Claude LLM."""

import json
import logging
import os
from typing import AsyncIterator

import sys
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..'))

from common.moltbot_agent import MoltbotAgent, AgentConfig
from common.llm_client import ResearcherLLM

logger = logging.getLogger(__name__)


class ResearcherAgent(MoltbotAgent):
    """Research agent that provides web search and document analysis using Claude LLM."""

    def get_capabilities(self) -> list[str]:
        return ["web_search", "document_analysis", "fact_checking", "research"]

    async def handle_request(self, request: dict) -> AsyncIterator[dict]:
        """Handle research request with Claude LLM."""
        query = request.get("query", request.get("text", ""))
        depth = request.get("depth", "standard")  # quick, standard, detailed

        if not query:
            yield {
                "type": "result",
                "parts": [{
                    "type": "text",
                    "text": json.dumps({
                        "error": "No query provided",
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
            "message": f"Researching: {query[:50]}..."
        }

        yield {
            "type": "status",
            "state": "working",
            "message": "Querying Claude LLM for research..."
        }

        # Use specialized research method
        if isinstance(self.llm_client, ResearcherLLM):
            result = await self.llm_client.research(query, depth=depth)
            research_result = {
                "query": query,
                "depth": depth,
                "llm_response": result.get("response", ""),
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
                prompt=f"Research the following topic and provide key findings: {query}",
                system="You are an AI research assistant. Provide thorough, accurate research with cited sources."
            )
            research_result = {
                "query": query,
                "llm_response": response,
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
                "text": json.dumps(research_result, indent=2)
            }]
        }


def create_agent() -> ResearcherAgent:
    """Create and configure the researcher agent."""
    config = AgentConfig(
        agent_id="moltbot-researcher",
        agent_name="Moltbot Researcher",
        description="AI research agent providing web search, document analysis, and fact-checking services",
        port=int(os.environ.get("PORT", "8095")),
        initial_tokens=float(os.environ.get("INITIAL_TOKENS", "50")),
        service_price=float(os.environ.get("SERVICE_PRICE", "10")),
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
        agent_type="researcher",
    )
    return ResearcherAgent(config=config)

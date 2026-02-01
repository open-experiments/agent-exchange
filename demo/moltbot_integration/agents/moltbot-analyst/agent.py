"""Moltbot Analyst Agent - Provides data analysis services for tokens using Claude LLM.

This agent demonstrates the purchase flow:
1. Receives analysis request from user/other agent
2. Needs research data → BUYS from Researcher agent (10 AEX)
3. Needs formatted report → BUYS from Writer agent (15 AEX)
4. Delivers final analysis → EARNS payment (20 AEX)
"""

import json
import logging
import os
from typing import AsyncIterator

import sys
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..'))

from common.moltbot_agent import MoltbotAgent, AgentConfig
from common.llm_client import AnalystLLM

logger = logging.getLogger(__name__)


class AnalystAgent(MoltbotAgent):
    """Analyst agent that provides data analysis and insights services using Claude LLM.

    PURCHASE TRIGGERS:
    - When handling a "comprehensive" analysis → buys research from Researcher
    - When requested report format → buys writing from Writer
    - When balance is low → refuses expensive operations
    """

    def get_capabilities(self) -> list[str]:
        return ["data_analysis", "statistics", "visualization", "insights", "reporting"]

    async def _buy_research(self, topic: str) -> dict:
        """Buy research data from the Researcher agent."""
        logger.info(f"Analyst needs research on: {topic}")

        # Check if we can afford research (10 AEX)
        if not await self.can_afford(10):
            logger.warning("Cannot afford research service")
            return {"error": "Insufficient funds for research", "needed": 10}

        # Try to find and pay the researcher
        try:
            result = await self.request_service_from_agent(
                target_agent="moltbot-researcher",
                request_data={"query": topic, "depth": "detailed"},
                pay_upfront=True,  # PAY 10 AEX before getting service
            )
            if result:
                logger.info(f"Purchased research for 10 AEX: {topic}")
                return result
        except Exception as e:
            logger.error(f"Failed to buy research: {e}")

        return {"error": "Research service unavailable"}

    async def _buy_report_writing(self, data: dict) -> dict:
        """Buy report writing from the Writer agent."""
        logger.info("Analyst needs report written")

        # Check if we can afford writing (15 AEX)
        if not await self.can_afford(15):
            logger.warning("Cannot afford writing service")
            return {"error": "Insufficient funds for writing", "needed": 15}

        # Try to find and pay the writer
        try:
            result = await self.request_service_from_agent(
                target_agent="moltbot-writer",
                request_data={"content": data, "format": "executive_report"},
                pay_upfront=True,  # PAY 15 AEX before getting service
            )
            if result:
                logger.info("Purchased report writing for 15 AEX")
                return result
        except Exception as e:
            logger.error(f"Failed to buy writing: {e}")

        return {"error": "Writing service unavailable"}

    async def handle_request(self, request: dict) -> AsyncIterator[dict]:
        """Handle analysis request with Claude LLM.

        PURCHASE TRIGGERS:
        - analysis_type="comprehensive" → Buy research (10 AEX)
        - include_report=True → Buy report writing (15 AEX)
        - analysis_type="market_research" → Buy both (25 AEX total)
        """
        data = request.get("data", request.get("text", ""))
        topic = request.get("topic", request.get("query", "general analysis"))
        analysis_type = request.get("type", "summary")
        include_report = request.get("include_report", False)

        # Track costs for this request
        total_cost = 0
        purchases = []

        if not data and not topic:
            yield {
                "type": "result",
                "parts": [{
                    "type": "text",
                    "text": json.dumps({
                        "error": "No data or topic provided for analysis",
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

        # ============================================
        # TRIGGER 1: Comprehensive analysis needs research
        # ============================================
        research_data = None
        if analysis_type in ["comprehensive", "market_research", "deep_dive"]:
            yield {
                "type": "status",
                "state": "working",
                "message": f"Need research data for {analysis_type}. Purchasing from Researcher..."
            }

            # BUY RESEARCH (costs 10 AEX)
            research_data = await self._buy_research(topic)

            if "error" not in research_data:
                total_cost += 10
                purchases.append({
                    "service": "research",
                    "provider": "moltbot-researcher",
                    "cost": 10,
                    "status": "purchased"
                })
                logger.info(f"Purchased research for 10 AEX. Total spent: {total_cost}")
            else:
                logger.warning(f"Research purchase failed: {research_data.get('error')}")
                purchases.append({
                    "service": "research",
                    "provider": "moltbot-researcher",
                    "cost": 0,
                    "status": "failed",
                    "reason": research_data.get("error")
                })

        # Perform analysis with LLM
        yield {
            "type": "status",
            "state": "working",
            "message": f"Performing {analysis_type} analysis with Claude LLM..."
        }

        # Prepare analysis input
        analysis_input = f"Topic: {topic}"
        if data:
            analysis_input += f"\nData: {data}"
        if research_data and "error" not in research_data:
            analysis_input += f"\nResearch: {json.dumps(research_data)}"

        # Use specialized analyst method
        if isinstance(self.llm_client, AnalystLLM):
            result = await self.llm_client.analyze(analysis_input, analysis_type=analysis_type)
            analysis_result = {
                "analysis_type": analysis_type,
                "topic": topic,
                "llm_analysis": result.get("analysis", ""),
                "research_used": research_data is not None and "error" not in (research_data or {}),
                "model": self.config.llm_model,
            }
        else:
            # Fallback to generic LLM
            response = await self.llm_client.generate(
                prompt=f"Analyze the following and provide insights:\n{analysis_input}",
                system="You are an AI data analyst. Provide clear analysis with actionable recommendations."
            )
            analysis_result = {
                "analysis_type": analysis_type,
                "topic": topic,
                "llm_analysis": response,
                "research_used": research_data is not None and "error" not in (research_data or {}),
                "model": self.config.llm_model,
            }

        # ============================================
        # TRIGGER 2: Report formatting needs writer
        # ============================================
        if include_report or analysis_type == "executive":
            yield {
                "type": "status",
                "state": "working",
                "message": "Generating executive report. Purchasing writing service..."
            }

            # BUY REPORT WRITING (costs 15 AEX)
            report_result = await self._buy_report_writing(analysis_result)

            if "error" not in report_result:
                total_cost += 15
                purchases.append({
                    "service": "report_writing",
                    "provider": "moltbot-writer",
                    "cost": 15,
                    "status": "purchased"
                })
                analysis_result["formatted_report"] = report_result
                logger.info(f"Purchased report writing for 15 AEX. Total spent: {total_cost}")
            else:
                purchases.append({
                    "service": "report_writing",
                    "provider": "moltbot-writer",
                    "cost": 0,
                    "status": "failed",
                    "reason": report_result.get("error")
                })

        # Add cost breakdown to result
        analysis_result["cost_breakdown"] = {
            "analyst_fee": self.config.service_price,
            "subcontractor_costs": total_cost,
            "purchases": purchases,
            "total_cost_to_client": self.config.service_price,
            "analyst_profit": self.config.service_price - total_cost,
        }
        analysis_result["provider"] = {
            "agent_id": self.config.agent_id,
            "agent_name": self.config.agent_name,
            "balance_after": await self.get_balance(),
        }

        yield {
            "type": "result",
            "parts": [{
                "type": "text",
                "text": json.dumps(analysis_result, indent=2)
            }]
        }


def create_agent() -> AnalystAgent:
    """Create and configure the analyst agent."""
    config = AgentConfig(
        agent_id="moltbot-analyst",
        agent_name="Moltbot Analyst",
        description="AI analyst agent providing data analysis, statistics, visualization, and insights services",
        port=int(os.environ.get("PORT", "8097")),
        initial_tokens=float(os.environ.get("INITIAL_TOKENS", "100")),
        service_price=float(os.environ.get("SERVICE_PRICE", "20")),
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
        agent_type="analyst",
    )
    return AnalystAgent(config=config)

"""Legal Agent A (Budget) - Fast, affordable contract review using Claude."""

import logging
import os
from dataclasses import dataclass, field
from typing import Any, Optional

from langchain_anthropic import ChatAnthropic
from langchain_core.messages import HumanMessage, SystemMessage
from langgraph.graph import StateGraph, END

import sys
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from common.base_agent import BaseAgent, AgentState
from common.config import AgentConfig

logger = logging.getLogger(__name__)

# Budget tier prompts - concise and fast
CONTRACT_REVIEW_PROMPT = """You are a quick contract reviewer providing CONCISE analysis.
Keep responses SHORT and focused on the most critical issues only.

Provide:
1. 3-5 key risks (one line each)
2. Top 3 action items
3. Overall risk rating (Low/Medium/High)

Be direct. No lengthy explanations. Speed is priority."""

LEGAL_RESEARCH_PROMPT = """You are a legal researcher providing QUICK summaries.
Keep responses SHORT and actionable.

Provide:
1. Key regulations (bullet points)
2. Main compliance requirements
3. Immediate action items

Be concise. No detailed explanations."""


@dataclass
class LegalAgentA(BaseAgent):
    """Budget Legal Agent using Claude for fast, affordable analysis."""

    llm: Optional[ChatAnthropic] = field(default=None, init=False)

    def _setup_llm(self):
        """Initialize Claude LLM."""
        api_key = os.environ.get("ANTHROPIC_API_KEY")
        if not api_key:
            logger.warning("ANTHROPIC_API_KEY not set, using mock responses")
            self.llm = None
            return

        self.llm = ChatAnthropic(
            model=self.config.llm.model,
            temperature=self.config.llm.temperature,
            max_tokens=self.config.llm.max_tokens,
            anthropic_api_key=api_key,
        )
        logger.info(f"Initialized Claude LLM (Budget): {self.config.llm.model}")

    def _build_graph(self):
        """Build the LangGraph workflow."""
        self._graph = StateGraph(AgentState)

    def _detect_skill(self, content: str) -> str:
        """Detect which skill to use based on content."""
        content_lower = content.lower()

        contract_keywords = ["contract", "agreement", "terms", "clause", "review"]
        if any(kw in content_lower for kw in contract_keywords):
            return "contract_review"

        return "legal_research"

    async def process(self, state: AgentState) -> AgentState:
        """Process the legal request through Gemini (fast mode)."""
        messages = state["messages"]
        if not messages:
            state["result"] = "No message provided."
            return state

        user_content = messages[-1].get("content", "")
        skill = self._detect_skill(user_content)

        system_prompt = (
            CONTRACT_REVIEW_PROMPT if skill == "contract_review"
            else LEGAL_RESEARCH_PROMPT
        )

        if self.llm is None:
            state["result"] = self._mock_response(skill, user_content)
            state["artifacts"] = [{
                "name": f"{skill}_quick_analysis.txt",
                "parts": [{"type": "text", "text": state["result"]}],
            }]
            return state

        try:
            response = await self.llm.ainvoke([
                SystemMessage(content=system_prompt),
                HumanMessage(content=user_content),
            ])

            result = response.content
            state["result"] = result
            state["artifacts"] = [{
                "name": f"{skill}_quick_analysis.txt",
                "parts": [{"type": "text", "text": result}],
            }]

        except Exception as e:
            logger.exception(f"Error calling Claude: {e}")
            state["result"] = f"Error processing request: {str(e)}"

        return state

    def _mock_response(self, skill: str, content: str) -> str:
        """Generate mock response for testing (budget tier - concise)."""
        if skill == "contract_review":
            return """## Quick Contract Review

**Risk Rating: MEDIUM**

### Key Risks:
- Liability cap too low for contract value
- Auto-renewal clause favors other party
- IP ownership ambiguous

### Action Items:
1. Negotiate higher liability cap
2. Add 30-day termination notice
3. Clarify IP assignment

*Budget analysis - $10 | ~5 min*"""
        else:
            return """## Quick Legal Summary

### Key Regulations:
- GDPR applies (EU data)
- Standard contract law
- Industry compliance required

### Actions Needed:
1. Add privacy policy
2. Update data handling
3. Review annually

*Budget analysis - $10 | ~5 min*"""

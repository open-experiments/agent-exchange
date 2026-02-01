"""Legal Agent B (Standard) - Balanced contract review using Claude."""

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

# Standard tier prompts - balanced detail
CONTRACT_REVIEW_PROMPT = """You are an experienced contract attorney providing thorough analysis.

Provide a COMPREHENSIVE review including:

1. **Executive Summary** (2-3 sentences)
2. **Key Terms Analysis**
   - Payment terms and conditions
   - Duration and renewal provisions
   - Termination rights
3. **Risk Assessment**
   - Identify all material risks
   - Rate each risk (Low/Medium/High)
   - Explain potential impact
4. **Party Obligations**
   - Your client's obligations
   - Counterparty obligations
5. **Recommendations**
   - Specific clauses to negotiate
   - Suggested modifications
   - Deal breakers to watch for

Be thorough but organized. Use tables where helpful."""

COMPLIANCE_CHECK_PROMPT = """You are a compliance specialist providing detailed regulatory analysis.

Provide a COMPREHENSIVE compliance assessment:

1. **Regulatory Framework**
   - Applicable laws and regulations
   - Jurisdictional considerations
2. **Compliance Checklist**
   - Map requirements to document sections
   - Identify gaps and violations
3. **Risk Matrix**
   - Categorize by severity
   - Estimate remediation effort
4. **Action Plan**
   - Prioritized remediation steps
   - Timeline recommendations

Be specific about which regulations apply and why."""

LEGAL_RESEARCH_PROMPT = """You are a legal researcher providing in-depth analysis.

Provide THOROUGH research including:

1. **Legal Framework** - Applicable laws and statutes
2. **Key Requirements** - Detailed compliance obligations
3. **Case Context** - Relevant precedents if applicable
4. **Practical Implications** - How this affects the client
5. **Recommendations** - Specific next steps

Include citations where possible."""


@dataclass
class LegalAgentB(BaseAgent):
    """Standard Legal Agent using Claude for balanced, thorough analysis."""

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
            api_key=api_key,
        )
        logger.info(f"Initialized Claude LLM (Standard): {self.config.llm.model}")

    def _build_graph(self):
        """Build the LangGraph workflow."""
        self._graph = StateGraph(AgentState)

    def _detect_skill(self, content: str) -> str:
        """Detect which skill to use based on content."""
        content_lower = content.lower()

        compliance_keywords = [
            "compliance", "gdpr", "ccpa", "hipaa", "regulation",
            "privacy", "data protection", "audit"
        ]
        if any(kw in content_lower for kw in compliance_keywords):
            return "compliance_check"

        research_keywords = ["research", "law", "statute", "requirement"]
        if any(kw in content_lower for kw in research_keywords):
            return "legal_research"

        return "contract_review"

    async def process(self, state: AgentState) -> AgentState:
        """Process the legal request through Claude (standard mode)."""
        messages = state["messages"]
        if not messages:
            state["result"] = "No message provided."
            return state

        user_content = messages[-1].get("content", "")
        skill = self._detect_skill(user_content)

        prompts = {
            "contract_review": CONTRACT_REVIEW_PROMPT,
            "compliance_check": COMPLIANCE_CHECK_PROMPT,
            "legal_research": LEGAL_RESEARCH_PROMPT,
        }
        system_prompt = prompts.get(skill, CONTRACT_REVIEW_PROMPT)

        if self.llm is None:
            state["result"] = self._mock_response(skill, user_content)
            state["artifacts"] = [{
                "name": f"{skill}_standard_report.txt",
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
                "name": f"{skill}_standard_report.txt",
                "parts": [{"type": "text", "text": result}],
            }]

        except Exception as e:
            logger.exception(f"Error calling Claude: {e}")
            state["result"] = f"Error processing request: {str(e)}"

        return state

    def _mock_response(self, skill: str, content: str) -> str:
        """Generate mock response for testing (standard tier - balanced)."""
        if skill == "contract_review":
            return """## Contract Review Report

### Executive Summary
This service agreement establishes a 12-month engagement with standard commercial terms. Several provisions require attention before signing, particularly around liability and IP ownership.

### Key Terms Analysis

| Term | Current State | Assessment |
|------|--------------|------------|
| **Payment** | Net-30, milestone-based | Standard, acceptable |
| **Duration** | 12 months, auto-renewal | Add termination notice period |
| **Liability** | Capped at contract value | Consider increasing cap |
| **IP Rights** | Shared ownership | Needs clarification |
| **Confidentiality** | Mutual, 3-year term | Standard |

### Risk Assessment

| Risk | Level | Impact | Mitigation |
|------|-------|--------|------------|
| Liability exposure | **Medium** | Financial | Negotiate higher cap |
| IP ownership dispute | **Medium** | Operational | Define ownership clearly |
| Auto-renewal lock-in | **Low** | Flexibility | Add 60-day notice |
| Payment delays | **Low** | Cash flow | Standard terms |

### Party Obligations

**Your Obligations:**
- Deliver services per SOW
- Maintain confidentiality
- Provide qualified personnel

**Counterparty Obligations:**
- Timely payment
- Reasonable cooperation
- Access to necessary resources

### Recommendations

1. **Negotiate**: Increase liability cap to 2x contract value
2. **Clarify**: Add specific IP assignment clause
3. **Add**: 60-day termination notice requirement
4. **Review**: Insurance requirements

*Standard analysis - $25 | ~15 min*"""
        else:
            return """## Compliance Assessment Report

### Regulatory Framework

**Applicable Regulations:**
- GDPR (EU General Data Protection Regulation)
- Local data protection laws
- Industry-specific requirements

### Compliance Checklist

| Requirement | Status | Evidence | Gap |
|------------|--------|----------|-----|
| Lawful basis for processing | Partial | Section 3.2 | Missing explicit consent |
| Data subject rights | Compliant | Section 5 | None |
| Data retention policy | Non-compliant | Missing | No policy documented |
| Security measures | Partial | Section 7 | Technical measures unclear |
| Breach notification | Compliant | Section 8 | None |

### Risk Matrix

| Issue | Severity | Likelihood | Remediation Effort |
|-------|----------|------------|-------------------|
| Missing retention policy | **High** | Likely | Medium (2-3 days) |
| Unclear consent | **Medium** | Possible | Low (1 day) |
| Security documentation | **Low** | Unlikely | Low (1 day) |

### Action Plan

**Immediate (Week 1):**
1. Draft data retention policy
2. Update consent mechanisms

**Short-term (Month 1):**
3. Document security measures
4. Conduct gap assessment

*Standard analysis - $25 | ~15 min*"""

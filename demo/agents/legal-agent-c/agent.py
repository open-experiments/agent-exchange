"""Legal Agent C (Premium) - Expert-level contract review using Claude."""

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

# Premium tier prompts - exhaustive expert analysis
CONTRACT_REVIEW_PROMPT = """You are a senior partner at a top law firm providing EXHAUSTIVE contract analysis.

Deliver EXPERT-LEVEL analysis including:

1. **Executive Summary** - Strategic overview for C-suite
2. **Deal Structure Analysis**
   - Commercial terms breakdown
   - Value assessment
   - Market comparison
3. **Comprehensive Risk Matrix**
   - ALL identified risks categorized by:
     * Legal risk
     * Financial risk
     * Operational risk
     * Reputational risk
   - Probability and impact scoring
   - Cumulative risk exposure
4. **Clause-by-Clause Analysis**
   - Every material clause reviewed
   - Standard vs. non-standard terms
   - Hidden traps and gotchas
5. **Negotiation Strategy**
   - Leverage points
   - Walk-away triggers
   - Alternative clause language
   - Fallback positions
6. **Regulatory Considerations**
   - Applicable laws
   - Jurisdictional issues
   - Future regulatory risks
7. **Precedent Analysis**
   - Similar deals reviewed
   - Industry standards
8. **Strategic Recommendations**
   - Prioritized negotiation points
   - Deal breakers
   - Accept/reject recommendation

This is board-level analysis. Be exhaustive. Miss nothing."""

COMPLIANCE_CHECK_PROMPT = """You are a chief compliance officer providing EXHAUSTIVE regulatory analysis.

Deliver EXPERT-LEVEL compliance assessment:

1. **Executive Summary** - Board-level compliance status
2. **Multi-Jurisdictional Analysis**
   - All applicable regulations by jurisdiction
   - Conflicts of law analysis
   - Extraterritorial implications
3. **Comprehensive Compliance Matrix**
   - Every requirement mapped
   - Evidence assessment
   - Gap severity scoring
4. **Risk Quantification**
   - Penalty exposure calculations
   - Enforcement likelihood
   - Reputational impact assessment
5. **Remediation Roadmap**
   - Detailed action items
   - Resource requirements
   - Timeline with milestones
   - Budget estimates
6. **Governance Recommendations**
   - Policy updates needed
   - Training requirements
   - Monitoring systems
7. **Audit Trail Documentation**
   - Evidence preservation
   - Documentation requirements

This is for regulatory filing purposes. Be exhaustive."""

LEGAL_RESEARCH_PROMPT = """You are a senior legal researcher providing EXHAUSTIVE analysis.

Deliver EXPERT-LEVEL research:

1. **Legal Framework** - Complete statutory landscape
2. **Case Law Analysis** - Relevant precedents with citations
3. **Regulatory Guidance** - Agency interpretations
4. **Comparative Analysis** - Multi-jurisdictional view
5. **Risk Assessment** - Litigation exposure
6. **Strategic Recommendations** - Actionable guidance

Include full citations. This is litigation-grade research."""

NEGOTIATION_PROMPT = """You are a senior negotiation strategist.

Provide COMPREHENSIVE negotiation support:

1. **Leverage Analysis** - Your strengths and weaknesses
2. **Counterparty Assessment** - Their likely positions
3. **Alternative Clauses** - Draft language options
4. **BATNA Analysis** - Best alternatives
5. **Concession Strategy** - What to give, what to hold
6. **Deal Structure Options** - Creative solutions

Be strategic and thorough."""


@dataclass
class LegalAgentC(BaseAgent):
    """Premium Legal Agent using Claude for exhaustive, expert-level analysis."""

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
        logger.info(f"Initialized Claude LLM (Premium): {self.config.llm.model}")

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

        negotiation_keywords = ["negotiate", "negotiation", "leverage", "strategy"]
        if any(kw in content_lower for kw in negotiation_keywords):
            return "negotiation_support"

        research_keywords = ["research", "law", "statute", "case", "precedent"]
        if any(kw in content_lower for kw in research_keywords):
            return "legal_research"

        return "contract_review"

    async def process(self, state: AgentState) -> AgentState:
        """Process the legal request through Claude (premium mode)."""
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
            "negotiation_support": NEGOTIATION_PROMPT,
        }
        system_prompt = prompts.get(skill, CONTRACT_REVIEW_PROMPT)

        if self.llm is None:
            state["result"] = self._mock_response(skill, user_content)
            state["artifacts"] = [{
                "name": f"{skill}_premium_report.txt",
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
                "name": f"{skill}_premium_report.txt",
                "parts": [{"type": "text", "text": result}],
            }]

        except Exception as e:
            logger.exception(f"Error calling Claude: {e}")
            state["result"] = f"Error processing request: {str(e)}"

        return state

    def _mock_response(self, skill: str, content: str) -> str:
        """Generate mock response for testing (premium tier - exhaustive)."""
        if skill == "contract_review":
            return """## Premium Contract Analysis Report
### Prepared by Elite Legal Partners

---

## 1. Executive Summary

This Master Service Agreement presents a **moderate-to-high risk profile** requiring significant negotiation before execution. Key concerns include liability limitations, IP ownership ambiguity, and one-sided termination provisions. **Recommendation: Do not sign without amendments.**

---

## 2. Deal Structure Analysis

### 2.1 Commercial Terms

| Element | Terms | Market Comparison | Assessment |
|---------|-------|-------------------|------------|
| Contract Value | $500,000/year | Market rate | Fair |
| Payment Terms | Net-30 | Industry: Net-45 | Favorable |
| Term | 36 months | Typical: 12-24 | Long commitment |
| Renewal | Auto-renewal | Concerning | Negotiate |

### 2.2 Value Assessment
- **Total Contract Value**: $1.5M over term
- **Maximum Liability Exposure**: $500K (capped)
- **Hidden Costs Identified**: ~$75K in compliance requirements

---

## 3. Comprehensive Risk Matrix

### 3.1 Legal Risks

| Risk | Probability | Impact | Score | Mitigation |
|------|------------|--------|-------|------------|
| IP ownership dispute | High | Critical | **9/10** | Rewrite Section 8 |
| Indemnification trigger | Medium | High | **7/10** | Cap at contract value |
| Regulatory non-compliance | Medium | Medium | **5/10** | Add compliance rep |
| Jurisdiction risk | Low | High | **4/10** | Arbitration clause |

### 3.2 Financial Risks

| Risk | Exposure | Likelihood | Annual Impact |
|------|----------|------------|---------------|
| Early termination penalty | $250K | 20% | $50K |
| Uncapped liability event | Unlimited | 5% | Unknown |
| Payment dispute | $150K | 15% | $22.5K |

### 3.3 Cumulative Risk Exposure
**Estimated Maximum Exposure: $1.2M**
**Expected Annual Risk Cost: $95K**

---

## 4. Clause-by-Clause Analysis

### Section 1: Definitions
- **Status**: Standard
- **Issues**: None material
- **Action**: Accept as-is

### Section 3: Scope of Services
- **Status**: Non-standard
- **Issues**: Scope creep language in 3.4(b)
- **Action**: Add change order requirement

### Section 5: Payment Terms
- **Status**: Favorable
- **Issues**: Late payment interest excessive (18%)
- **Action**: Negotiate to 12%

### Section 7: Limitation of Liability
- **Status**: CRITICAL CONCERN
- **Issues**:
  - Cap too low ($100K vs. $500K contract)
  - Carve-outs favor counterparty
  - No mutual limitation
- **Action**: MUST renegotiate before signing

### Section 8: Intellectual Property
- **Status**: CRITICAL CONCERN
- **Issues**:
  - Work product ownership unclear
  - Background IP not protected
  - License-back provisions missing
- **Action**: Complete rewrite required

### Section 12: Termination
- **Status**: One-sided
- **Issues**:
  - Counterparty: 30-day convenience termination
  - Your termination: Only for material breach
- **Action**: Add mutual convenience termination

---

## 5. Negotiation Strategy

### 5.1 Leverage Points
1. Market alternatives available
2. Long-term commitment being offered
3. Counterparty needs this deal for Q4 numbers

### 5.2 Priority Negotiation Items

| Priority | Item | Current | Target | Fallback |
|----------|------|---------|--------|----------|
| 1 | Liability cap | $100K | $1M | $500K |
| 2 | IP ownership | Unclear | Full ownership | License-back |
| 3 | Termination | One-sided | Mutual 90-day | Mutual 60-day |
| 4 | Auto-renewal | Silent | 90-day notice | 60-day notice |

### 5.3 Walk-Away Triggers
- Liability cap below $250K
- No IP ownership clarity
- Unlimited indemnification

### 5.4 Alternative Clause Language

**Proposed Section 7.1 (Liability):**
> "Each party's aggregate liability under this Agreement shall not exceed two times (2x) the total fees paid or payable during the twelve (12) month period immediately preceding the claim, provided that this limitation shall not apply to: (a) breaches of confidentiality, (b) willful misconduct, or (c) indemnification obligations."

---

## 6. Regulatory Considerations

- **GDPR**: Data processing addendum required
- **CCPA**: Privacy notice updates needed
- **SOC 2**: Audit rights should be added
- **Industry Regs**: Sector-specific compliance TBD

---

## 7. Strategic Recommendations

### Immediate Actions
1. **DO NOT SIGN** current version
2. Send redline with priority amendments
3. Schedule negotiation call for this week

### Deal Recommendation
**Proceed with caution** - Deal is commercially attractive but legal terms require significant modification. Estimated negotiation time: 2-3 weeks.

---

*Premium analysis - $50 | ~30 min*
*Prepared by Elite Legal Partners - Confidential*"""
        else:
            return """## Premium Compliance Assessment Report
### Prepared by Elite Legal Partners

---

## 1. Executive Summary

**Overall Compliance Status: HIGH RISK**

This assessment identifies **12 compliance gaps** across **4 regulatory frameworks**, with an estimated penalty exposure of **$2.4M**. Immediate remediation required before regulatory audit.

---

## 2. Multi-Jurisdictional Analysis

### 2.1 Applicable Regulations

| Regulation | Jurisdiction | Applicability | Risk Level |
|------------|-------------|---------------|------------|
| GDPR | EU/EEA | **Full** | Critical |
| CCPA/CPRA | California | **Full** | High |
| LGPD | Brazil | Partial | Medium |
| PIPEDA | Canada | Partial | Low |

### 2.2 Extraterritorial Implications
- EU data subjects trigger GDPR regardless of processing location
- California revenue threshold exceeded - CPRA applies
- Brazilian subsidiaries trigger LGPD

---

## 3. Comprehensive Compliance Matrix

### 3.1 GDPR Compliance

| Article | Requirement | Status | Evidence | Gap Severity |
|---------|-------------|--------|----------|--------------|
| Art. 6 | Lawful basis | Partial | Privacy Policy 3.2 | **High** |
| Art. 7 | Consent | Non-compliant | Missing | **Critical** |
| Art. 13-14 | Transparency | Partial | Incomplete | **Medium** |
| Art. 15-22 | Data subject rights | Compliant | Portal exists | None |
| Art. 25 | Privacy by design | Non-compliant | No documentation | **High** |
| Art. 30 | Processing records | Non-compliant | Missing | **High** |
| Art. 32 | Security measures | Partial | SOC 2 pending | **Medium** |
| Art. 33-34 | Breach notification | Compliant | Policy exists | None |

### 3.2 Overall Gap Score: 62/100 (High Risk)

---

## 4. Risk Quantification

### 4.1 Penalty Exposure

| Regulation | Max Penalty | Likely Penalty | Probability | Expected Cost |
|------------|-------------|----------------|-------------|---------------|
| GDPR | 4% revenue (~$8M) | $1.5M | 30% | $450K |
| CCPA | $7,500/violation | $500K | 40% | $200K |
| Class action | N/A | $2M | 15% | $300K |
| **Total** | | | | **$950K** |

### 4.2 Non-Monetary Risks
- Regulatory investigation: 6-12 months disruption
- Reputational damage: Customer churn estimated 5-8%
- M&A impact: Due diligence red flag

---

## 5. Remediation Roadmap

### Phase 1: Critical (0-30 days) - Budget: $75K

| # | Action | Owner | Deadline | Cost |
|---|--------|-------|----------|------|
| 1 | Implement consent management | Privacy | Day 14 | $25K |
| 2 | Create processing records | Legal | Day 21 | $15K |
| 3 | Update privacy notices | Marketing | Day 30 | $10K |
| 4 | Privacy impact assessment | Privacy | Day 30 | $25K |

### Phase 2: High Priority (30-90 days) - Budget: $150K

| # | Action | Owner | Deadline | Cost |
|---|--------|-------|----------|------|
| 5 | Privacy by design framework | Engineering | Day 60 | $50K |
| 6 | Data mapping exercise | IT | Day 75 | $40K |
| 7 | Vendor assessment program | Procurement | Day 90 | $30K |
| 8 | Training program rollout | HR | Day 90 | $30K |

### Phase 3: Maintenance (90+ days) - Budget: $50K/year

| # | Action | Frequency | Cost |
|---|--------|-----------|------|
| 9 | Annual compliance audit | Yearly | $25K |
| 10 | Continuous monitoring | Ongoing | $25K |

---

## 6. Governance Recommendations

### 6.1 Policy Updates Required
1. Data Retention Policy - NEW
2. Privacy Policy - MAJOR UPDATE
3. Incident Response Plan - MINOR UPDATE
4. Vendor Management Policy - NEW

### 6.2 Training Requirements
- All employees: Privacy fundamentals (2 hours)
- Engineering: Privacy by design (4 hours)
- Customer support: DSAR handling (3 hours)
- Leadership: Regulatory landscape (1 hour)

---

## 7. Board-Level Summary

**Compliance investment required: $275K (Year 1)**
**Risk reduction: 85% penalty exposure eliminated**
**ROI: 3.5x based on expected cost avoidance**

---

*Premium analysis - $50 | ~30 min*
*Prepared by Elite Legal Partners - Confidential*"""

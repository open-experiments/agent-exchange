"""Orchestrator Agent - Task decomposition and multi-agent coordination."""

import asyncio
import json
import logging
import os
from dataclasses import dataclass, field
from typing import Any, Optional
import aiohttp

from langchain_anthropic import ChatAnthropic
from langchain_core.messages import HumanMessage, SystemMessage
from langgraph.graph import StateGraph, END

import sys
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from common.base_agent import BaseAgent, AgentState
from common.config import AgentConfig
from common.aex_client import AEXClient

logger = logging.getLogger(__name__)

ORCHESTRATOR_PROMPT = """You are an intelligent orchestrator that decomposes complex user requests into subtasks.

Given a user request, identify the required subtasks and the skills needed for each.
Output a JSON object with the following structure:

{
    "understanding": "Brief summary of what the user wants",
    "subtasks": [
        {
            "id": "task_1",
            "description": "What needs to be done",
            "skill_tags": ["skill_tag_1", "skill_tag_2"],
            "input": "Specific input for this subtask",
            "depends_on": []
        }
    ],
    "execution_order": "parallel" or "sequential"
}

Available skill tags:
- contract_review: Review and analyze contracts (basic review)
- legal_research: Research legal topics and regulations
- compliance_check: Check regulatory compliance
- negotiation_support: Strategic negotiation advice and alternative clause suggestions
- risk_analysis: Detailed risk assessment and mitigation strategies (premium)
- flight_booking: Search and book flights
- hotel_booking: Search and book hotels
- itinerary_planning: Create travel itineraries

For complex legal requests, consider decomposing into:
1. Basic contract_review for quick overview
2. compliance_check for regulatory issues
3. negotiation_support for strategic advice on problematic clauses
4. risk_analysis for detailed risk assessment

Be specific about what each subtask should accomplish.
Return ONLY the JSON object, no additional text."""


@dataclass
class SubTask:
    """A subtask identified by the orchestrator."""
    id: str
    description: str
    skill_tags: list[str]
    input: str
    depends_on: list[str]
    result: Optional[str] = None
    status: str = "pending"
    provider_id: Optional[str] = None
    agent_url: Optional[str] = None


@dataclass
class OrchestratorAgent(BaseAgent):
    """Orchestrator that coordinates multiple agents via AEX + A2A."""

    llm: Optional[ChatAnthropic] = field(default=None, init=False)
    http_session: Optional[aiohttp.ClientSession] = field(default=None, init=False)

    def _setup_llm(self):
        """Initialize Claude LLM for task decomposition."""
        api_key = os.environ.get("ANTHROPIC_API_KEY")
        if not api_key:
            logger.warning("ANTHROPIC_API_KEY not set, using mock decomposition")
            self.llm = None
            return

        self.llm = ChatAnthropic(
            model=self.config.llm.model,
            temperature=self.config.llm.temperature,
            max_tokens=self.config.llm.max_tokens,
            api_key=api_key,
        )
        logger.info(f"Initialized Claude LLM: {self.config.llm.model}")

    def _build_graph(self):
        """Build the orchestration workflow."""
        self._graph = StateGraph(AgentState)

    async def process(self, state: AgentState) -> AgentState:
        """Process request through orchestration pipeline."""
        messages = state["messages"]
        if not messages:
            state["result"] = "No message provided."
            return state

        user_content = messages[-1].get("content", "")

        # Step 1: Decompose task
        subtasks = await self._decompose_task(user_content)
        if not subtasks:
            state["result"] = "Could not decompose the request into subtasks."
            return state

        logger.info(f"Decomposed into {len(subtasks)} subtasks")

        # Step 2: Discover providers via AEX for each subtask
        await self._discover_providers(subtasks)

        # Step 3: Execute subtasks via A2A
        results = await self._execute_subtasks(subtasks)

        # Step 4: Aggregate results
        state["result"] = self._aggregate_results(user_content, subtasks)
        state["artifacts"] = [
            {
                "name": "orchestration_report.json",
                "parts": [{"type": "text", "text": json.dumps({
                    "subtasks": [
                        {
                            "id": st.id,
                            "description": st.description,
                            "status": st.status,
                            "provider": st.provider_id,
                        }
                        for st in subtasks
                    ]
                }, indent=2)}],
            }
        ]

        return state

    async def _decompose_task(self, user_request: str) -> list[SubTask]:
        """Use LLM to decompose request into subtasks."""
        if self.llm is None:
            return self._mock_decompose(user_request)

        try:
            response = await self.llm.ainvoke([
                SystemMessage(content=ORCHESTRATOR_PROMPT),
                HumanMessage(content=user_request),
            ])

            # Parse JSON response
            content = response.content.strip()
            # Handle markdown code blocks
            if content.startswith("```"):
                content = content.split("```")[1]
                if content.startswith("json"):
                    content = content[4:]
                content = content.strip()

            data = json.loads(content)

            subtasks = []
            for st in data.get("subtasks", []):
                subtasks.append(SubTask(
                    id=st["id"],
                    description=st["description"],
                    skill_tags=st.get("skill_tags", []),
                    input=st.get("input", ""),
                    depends_on=st.get("depends_on", []),
                ))

            return subtasks

        except Exception as e:
            logger.exception(f"Error decomposing task: {e}")
            return self._mock_decompose(user_request)

    def _mock_decompose(self, user_request: str) -> list[SubTask]:
        """Mock decomposition for testing."""
        request_lower = user_request.lower()

        subtasks = []
        task_id = 1

        # Basic contract review (Budget agent)
        if "contract" in request_lower or "review" in request_lower or "nda" in request_lower:
            subtasks.append(SubTask(
                id=f"task_{task_id}",
                description="Review the contract for risks and obligations",
                skill_tags=["contract_review", "legal"],
                input=user_request,
                depends_on=[],
            ))
            task_id += 1

        # Compliance check (Standard agent)
        if "compliance" in request_lower or "regulation" in request_lower or "gdpr" in request_lower:
            subtasks.append(SubTask(
                id=f"task_{task_id}",
                description="Check regulatory compliance",
                skill_tags=["compliance_check", "legal"],
                input=user_request,
                depends_on=[],
            ))
            task_id += 1

        # Negotiation support (Premium agent)
        if "negotiate" in request_lower or "terms" in request_lower or "clause" in request_lower:
            subtasks.append(SubTask(
                id=f"task_{task_id}",
                description="Provide strategic negotiation recommendations",
                skill_tags=["negotiation_support", "legal"],
                input=user_request,
                depends_on=[],
            ))
            task_id += 1

        # Risk analysis (Premium agent)
        if "risk" in request_lower or "assessment" in request_lower or "analysis" in request_lower:
            subtasks.append(SubTask(
                id=f"task_{task_id}",
                description="Perform detailed risk analysis with mitigation strategies",
                skill_tags=["risk_analysis", "legal"],
                input=user_request,
                depends_on=[],
            ))
            task_id += 1

        # Travel tasks
        if "travel" in request_lower or "berlin" in request_lower or "flight" in request_lower:
            subtasks.append(SubTask(
                id=f"task_{task_id}",
                description="Find flight options",
                skill_tags=["flight_booking", "travel"],
                input=user_request,
                depends_on=[],
            ))
            task_id += 1
            subtasks.append(SubTask(
                id=f"task_{task_id}",
                description="Find hotel options",
                skill_tags=["hotel_booking", "travel"],
                input=user_request,
                depends_on=[],
            ))
            task_id += 1

        # Default: comprehensive review using all three agents
        if not subtasks:
            subtasks = [
                SubTask(
                    id="task_1",
                    description="Quick contract overview",
                    skill_tags=["contract_review", "legal"],
                    input=user_request,
                    depends_on=[],
                ),
                SubTask(
                    id="task_2",
                    description="Compliance verification",
                    skill_tags=["compliance_check", "legal"],
                    input=user_request,
                    depends_on=[],
                ),
                SubTask(
                    id="task_3",
                    description="Strategic risk analysis and recommendations",
                    skill_tags=["risk_analysis", "legal"],
                    input=user_request,
                    depends_on=[],
                ),
            ]

        return subtasks

    async def _discover_providers(self, subtasks: list[SubTask]):
        """Discover providers via AEX for each subtask."""
        # Demo agent URLs (Docker network hostnames)
        # Each agent has different specialties to demonstrate multi-agent coordination
        demo_agents = {
            # Budget agent - quick basic reviews
            "contract_review": ("http://legal-agent-a:8100", "Budget Legal Agent ($5+$2/page)"),
            "legal": ("http://legal-agent-a:8100", "Budget Legal Agent ($5+$2/page)"),
            # Standard agent - compliance and research
            "legal_research": ("http://legal-agent-b:8101", "Standard Legal Agent ($15+$0.50/page)"),
            "compliance_check": ("http://legal-agent-b:8101", "Standard Legal Agent ($15+$0.50/page)"),
            "compliance": ("http://legal-agent-b:8101", "Standard Legal Agent ($15+$0.50/page)"),
            # Premium agent - strategic advice and risk analysis
            "negotiation_support": ("http://legal-agent-c:8102", "Premium Legal Agent ($30+$0.20/page)"),
            "risk_analysis": ("http://legal-agent-c:8102", "Premium Legal Agent ($30+$0.20/page)"),
            "negotiation": ("http://legal-agent-c:8102", "Premium Legal Agent ($30+$0.20/page)"),
            "strategic": ("http://legal-agent-c:8102", "Premium Legal Agent ($30+$0.20/page)"),
        }

        for st in subtasks:
            # Try AEX discovery first
            if self.aex_client:
                try:
                    providers = await self.aex_client.search_providers(
                        skill_tags=st.skill_tags,
                    )
                    if providers:
                        st.provider_id = providers[0].get("provider_id")
                        st.agent_url = providers[0].get("endpoint")
                        logger.info(f"[AEX] Found provider {st.provider_id} for {st.id}")
                        continue
                except Exception as e:
                    logger.warning(f"[AEX] Discovery failed for {st.id}: {e}")

            # Fall back to demo agents
            for tag in st.skill_tags:
                if tag in demo_agents:
                    st.agent_url, st.provider_id = demo_agents[tag]
                    logger.info(f"[A2A] Using {st.provider_id} for subtask: {st.description}")
                    break

            # Default to budget legal agent
            if not st.agent_url:
                st.agent_url = "http://legal-agent-a:8100"
                st.provider_id = "Budget Legal Agent ($5+$2/page)"
                logger.info(f"[A2A] Default to {st.provider_id} for subtask: {st.description}")

    async def _execute_subtasks(self, subtasks: list[SubTask]) -> dict[str, str]:
        """Execute subtasks via A2A protocol."""
        results = {}

        async with aiohttp.ClientSession() as session:
            for st in subtasks:
                if not st.agent_url:
                    st.status = "failed"
                    st.result = "No provider available"
                    continue

                st.status = "running"
                try:
                    result = await self._call_a2a_agent(session, st)
                    st.result = result
                    st.status = "completed"
                    results[st.id] = result
                except Exception as e:
                    logger.exception(f"Error executing {st.id}: {e}")
                    st.status = "failed"
                    st.result = str(e)

        return results

    async def _call_a2a_agent(self, session: aiohttp.ClientSession, subtask: SubTask) -> str:
        """Call an agent via A2A JSON-RPC."""
        a2a_url = f"{subtask.agent_url}/a2a"
        logger.info(f"[A2A] Calling {subtask.provider_id} at {a2a_url}")

        payload = {
            "jsonrpc": "2.0",
            "method": "message/send",
            "id": subtask.id,
            "params": {
                "message": {
                    "role": "user",
                    "parts": [{"type": "text", "text": subtask.input}],
                }
            },
        }

        try:
            async with session.post(a2a_url, json=payload) as resp:
                if resp.status != 200:
                    error = await resp.text()
                    raise Exception(f"A2A call failed: {error}")

                data = await resp.json()

                if "error" in data:
                    raise Exception(data["error"].get("message", "Unknown error"))

                result = data.get("result", {})
                history = result.get("history", [])

                # Extract agent response
                for msg in reversed(history):
                    if msg.get("role") == "agent":
                        parts = msg.get("parts", [])
                        for part in parts:
                            if part.get("type") == "text":
                                return part.get("text", "")

                return "No response from agent"

        except aiohttp.ClientError as e:
            # If agent not reachable, return mock response
            logger.warning(f"Could not reach agent at {a2a_url}: {e}")
            return f"[Demo] Mock response for: {subtask.description}"

    def _aggregate_results(self, original_request: str, subtasks: list[SubTask]) -> str:
        """Aggregate results from all subtasks."""
        lines = ["# Orchestration Results\n"]
        lines.append(f"**Original Request**: {original_request}\n")
        lines.append(f"**Subtasks Executed**: {len(subtasks)}\n")

        # Agent Selection Summary
        lines.append("\n## 📊 Agent Selection Summary\n")
        lines.append("| Subtask | Skill Tags | Selected Agent | Selection Reason |")
        lines.append("|---------|------------|----------------|------------------|")

        for st in subtasks:
            tags_str = ", ".join(st.skill_tags[:2]) if st.skill_tags else "N/A"
            reason = self._get_selection_reason(st.skill_tags)
            lines.append(f"| {st.description[:40]}... | `{tags_str}` | {st.provider_id or 'Unknown'} | {reason} |")

        lines.append("\n---\n")

        for st in subtasks:
            status_icon = "✅" if st.status == "completed" else "❌"
            lines.append(f"\n## {status_icon} {st.description}")
            lines.append(f"**Provider**: {st.provider_id or 'Unknown'}")
            lines.append(f"**Skill Tags**: {', '.join(st.skill_tags)}")
            lines.append(f"**Status**: {st.status}\n")
            if st.result:
                lines.append(st.result)
            lines.append("\n---")

        return "\n".join(lines)

    def _get_selection_reason(self, skill_tags: list[str]) -> str:
        """Get reason for agent selection based on skill tags."""
        reasons = {
            "contract_review": "Basic review → Budget tier",
            "legal": "General legal → Budget tier",
            "compliance_check": "Compliance → Standard tier",
            "legal_research": "Research → Standard tier",
            "compliance": "Compliance → Standard tier",
            "negotiation_support": "Negotiation → Premium tier",
            "risk_analysis": "Risk analysis → Premium tier",
            "negotiation": "Strategic → Premium tier",
            "strategic": "Strategic → Premium tier",
        }
        for tag in skill_tags:
            if tag in reasons:
                return reasons[tag]
        return "Default routing"

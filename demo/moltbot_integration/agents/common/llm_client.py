"""LLM Client for AI-powered agent responses using Claude API.

This module provides real LLM responses for the Moltbot agents
using Anthropic's Claude API. Requires ANTHROPIC_API_KEY to function.
"""

import logging
import os
from dataclasses import dataclass, field
from typing import AsyncIterator, Optional

logger = logging.getLogger(__name__)

# Import anthropic - required dependency
try:
    import anthropic
    ANTHROPIC_AVAILABLE = True
except ImportError:
    ANTHROPIC_AVAILABLE = False
    logger.error("anthropic package not installed. Install with: pip install anthropic")


@dataclass
class LLMConfig:
    """Configuration for LLM client."""
    model: str = "claude-sonnet-4-20250514"  # Default to Claude Sonnet 4
    max_tokens: int = 1024
    temperature: float = 0.7
    api_key: str = ""  # Will try ANTHROPIC_API_KEY env var if empty

    def __post_init__(self):
        if not self.api_key:
            self.api_key = os.environ.get("ANTHROPIC_API_KEY", "")


@dataclass
class LLMClient:
    """Client for Claude API to generate real LLM responses.

    Usage:
        client = LLMClient(config=LLMConfig())
        response = await client.generate("What is AI?")
        # Or with streaming:
        async for chunk in client.generate_stream("What is AI?"):
            print(chunk, end="")
    """

    config: LLMConfig = field(default_factory=LLMConfig)
    _client: Optional["anthropic.AsyncAnthropic"] = field(default=None, init=False)

    def __post_init__(self):
        if ANTHROPIC_AVAILABLE and self.config.api_key:
            self._client = anthropic.AsyncAnthropic(api_key=self.config.api_key)

    @property
    def is_available(self) -> bool:
        """Check if LLM is available."""
        return self._client is not None

    async def generate(
        self,
        prompt: str,
        system: str = "",
        context: Optional[list[dict]] = None
    ) -> str:
        """Generate a response using Claude.

        Args:
            prompt: The user's message/query
            system: Optional system prompt for the agent's persona
            context: Optional conversation history as list of {"role": "user"|"assistant", "content": str}

        Returns:
            The generated response text

        Raises:
            RuntimeError: If LLM client is not available
        """
        if not self._client:
            raise RuntimeError("LLM client not available. Set ANTHROPIC_API_KEY environment variable.")

        try:
            messages = []

            # Add context if provided
            if context:
                messages.extend(context)

            # Add current prompt
            messages.append({"role": "user", "content": prompt})

            response = await self._client.messages.create(
                model=self.config.model,
                max_tokens=self.config.max_tokens,
                temperature=self.config.temperature,
                system=system if system else None,
                messages=messages,
            )

            # Extract text from response
            if response.content and len(response.content) > 0:
                return response.content[0].text
            return ""

        except anthropic.APIError as e:
            logger.error(f"Claude API error: {e}")
            raise RuntimeError(f"Claude API error: {str(e)}")
        except Exception as e:
            logger.error(f"LLM generation error: {e}")
            raise RuntimeError(f"LLM generation failed: {str(e)}")

    async def generate_stream(
        self,
        prompt: str,
        system: str = "",
        context: Optional[list[dict]] = None
    ) -> AsyncIterator[str]:
        """Generate a streaming response using Claude.

        Yields chunks of text as they're generated.

        Raises:
            RuntimeError: If LLM client is not available
        """
        if not self._client:
            raise RuntimeError("LLM client not available. Set ANTHROPIC_API_KEY environment variable.")

        try:
            messages = []
            if context:
                messages.extend(context)
            messages.append({"role": "user", "content": prompt})

            async with self._client.messages.stream(
                model=self.config.model,
                max_tokens=self.config.max_tokens,
                temperature=self.config.temperature,
                system=system if system else None,
                messages=messages,
            ) as stream:
                async for text in stream.text_stream:
                    yield text

        except anthropic.APIError as e:
            logger.error(f"Claude API error: {e}")
            raise RuntimeError(f"Claude API error: {str(e)}")
        except Exception as e:
            logger.error(f"LLM streaming error: {e}")
            raise RuntimeError(f"LLM streaming failed: {str(e)}")


# =============================================================================
# Agent-Specific LLM Helpers
# =============================================================================

class ResearcherLLM(LLMClient):
    """LLM client specialized for research tasks."""

    SYSTEM_PROMPT = """You are an AI research assistant specializing in web search,
document analysis, and fact-checking. You provide thorough, accurate research results
with cited sources. Be concise but comprehensive.

When given a research query:
1. Analyze the topic thoroughly
2. Provide key findings with supporting evidence
3. List sources and confidence levels
4. Highlight any uncertainties or areas needing more research"""

    async def research(self, query: str, depth: str = "standard") -> dict:
        """Perform research on a query.

        Args:
            query: The research topic or question
            depth: "quick", "standard", or "detailed"

        Returns:
            Research results as structured dict
        """
        depth_instructions = {
            "quick": "Provide a brief 2-3 point summary.",
            "standard": "Provide 4-5 key findings with moderate detail.",
            "detailed": "Provide comprehensive analysis with 6+ detailed findings.",
        }

        prompt = f"""Research Query: {query}

{depth_instructions.get(depth, depth_instructions['standard'])}

Format your response as:
- Finding 1: [Key insight]
- Finding 2: [Key insight]
...
- Sources: [List relevant source types]
- Confidence: [Low/Medium/High] - [Brief explanation]"""

        response = await self.generate(prompt, system=self.SYSTEM_PROMPT)

        return {
            "query": query,
            "depth": depth,
            "response": response,
            "provider": "claude-research-agent",
        }


class WriterLLM(LLMClient):
    """LLM client specialized for content writing."""

    SYSTEM_PROMPT = """You are an AI writing assistant specializing in copywriting,
technical documentation, and content creation. You produce clear, engaging,
well-structured content.

When given a writing task:
1. Understand the content type and audience
2. Structure the content appropriately
3. Use clear, engaging language
4. Include relevant sections and formatting"""

    async def write(
        self,
        topic: str,
        doc_type: str = "article",
        length: str = "medium"
    ) -> dict:
        """Write content on a topic.

        Args:
            topic: The topic to write about
            doc_type: "article", "report", "summary", "technical_doc"
            length: "short", "medium", "long"

        Returns:
            Written content as structured dict
        """
        length_instructions = {
            "short": "Write a brief piece (150-250 words).",
            "medium": "Write a moderate piece (300-500 words).",
            "long": "Write a comprehensive piece (600-1000 words).",
        }

        type_instructions = {
            "article": "Write an engaging article with introduction, body, and conclusion.",
            "report": "Write a formal report with executive summary and sections.",
            "summary": "Write a concise summary highlighting key points.",
            "technical_doc": "Write clear technical documentation with examples.",
        }

        prompt = f"""Topic: {topic}
Document Type: {doc_type}
Length: {length}

{type_instructions.get(doc_type, type_instructions['article'])}
{length_instructions.get(length, length_instructions['medium'])}

Include appropriate headings and structure."""

        response = await self.generate(prompt, system=self.SYSTEM_PROMPT)

        return {
            "topic": topic,
            "type": doc_type,
            "content": response,
            "provider": "claude-writer-agent",
        }


class AnalystLLM(LLMClient):
    """LLM client specialized for data analysis."""

    SYSTEM_PROMPT = """You are an AI data analyst specializing in statistics,
data processing, and generating insights. You provide clear analysis with
actionable recommendations.

When given data or a topic to analyze:
1. Identify key patterns and trends
2. Calculate relevant statistics
3. Generate visualizable insights
4. Provide recommendations"""

    async def analyze(
        self,
        data: str,
        analysis_type: str = "summary"
    ) -> dict:
        """Analyze data or a topic.

        Args:
            data: The data or topic to analyze
            analysis_type: "summary", "comprehensive", "market_research", "deep_dive"

        Returns:
            Analysis results as structured dict
        """
        type_instructions = {
            "summary": "Provide a high-level summary with 3-4 key insights.",
            "comprehensive": "Provide detailed analysis with metrics, trends, and recommendations.",
            "market_research": "Focus on market dynamics, competition, and opportunities.",
            "deep_dive": "Provide exhaustive analysis with all available angles.",
        }

        prompt = f"""Data/Topic to Analyze: {data}

{type_instructions.get(analysis_type, type_instructions['summary'])}

Format your response with:
- Key Metrics: [Relevant numbers/stats]
- Trends: [Identified patterns]
- Insights: [Actionable findings]
- Recommendations: [What to do next]"""

        response = await self.generate(prompt, system=self.SYSTEM_PROMPT)

        return {
            "input": data[:200] + "..." if len(data) > 200 else data,
            "analysis_type": analysis_type,
            "analysis": response,
            "provider": "claude-analyst-agent",
        }


# Factory function to create appropriate LLM client
def create_llm_client(
    agent_type: str,
    config: Optional[LLMConfig] = None
) -> LLMClient:
    """Create an LLM client for a specific agent type.

    Args:
        agent_type: "researcher", "writer", "analyst", or "general"
        config: Optional LLM configuration

    Returns:
        Appropriate LLM client instance
    """
    if config is None:
        config = LLMConfig()

    clients = {
        "researcher": ResearcherLLM,
        "writer": WriterLLM,
        "analyst": AnalystLLM,
    }

    client_class = clients.get(agent_type, LLMClient)
    return client_class(config=config)

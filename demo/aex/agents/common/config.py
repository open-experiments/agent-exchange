"""Configuration management for demo agents."""

from dataclasses import dataclass, field
from typing import Optional
import os
import yaml

from .agent_card import AgentCard, Skill, Provider, Capabilities, Extension, ap2_extension


@dataclass
class LLMConfig:
    """LLM configuration."""
    provider: str  # anthropic, openai, google
    model: str
    temperature: float = 0.7
    max_tokens: int = 4096


@dataclass
class ServerConfig:
    """Server configuration."""
    host: str = "0.0.0.0"
    port: int = 8100


@dataclass
class AEXConfig:
    """AEX integration configuration."""
    enabled: bool = True
    gateway_url: str = "http://localhost:8080"
    auto_register: bool = True
    auto_bid: bool = True
    base_rate: float = 25.0
    currency: str = "USD"
    ap2_enabled: bool = True  # Enable AP2 payment support
    trust_tier: str = "UNVERIFIED"  # UNVERIFIED, VERIFIED, TRUSTED, PREFERRED
    trust_score: float = 0.3  # 0.0 to 1.0


@dataclass
class AgentConfig:
    """Complete agent configuration."""
    name: str
    description: str
    version: str
    server: ServerConfig = field(default_factory=ServerConfig)
    llm: LLMConfig = field(default_factory=lambda: LLMConfig(provider="anthropic", model="claude-3-5-sonnet-20241022"))
    aex: AEXConfig = field(default_factory=AEXConfig)
    skills: list[Skill] = field(default_factory=list)
    provider: Optional[Provider] = None

    def get_agent_card(self, base_url: str) -> AgentCard:
        """Generate A2A Agent Card from config."""
        # Build extensions based on config
        extensions = []
        if self.aex.ap2_enabled:
            extensions.append(ap2_extension(required=True))

        return AgentCard(
            name=self.name,
            description=self.description,
            url=base_url,
            version=self.version,
            provider=self.provider,
            capabilities=Capabilities(streaming=True, extensions=extensions),
            skills=self.skills,
        )


def load_config(config_path: str = "config.yaml") -> AgentConfig:
    """Load agent configuration from YAML file."""
    with open(config_path, "r") as f:
        data = yaml.safe_load(f)

    agent_data = data.get("agent", {})
    server_data = data.get("server", {})
    llm_data = data.get("llm", {})
    aex_data = data.get("aex", {})
    skills_data = data.get("skills", [])

    # Parse skills
    skills = [
        Skill(
            id=s["id"],
            name=s["name"],
            description=s.get("description", ""),
            tags=s.get("tags", []),
            examples=s.get("examples", []),
        )
        for s in skills_data
    ]

    # Parse provider
    provider = None
    if "provider" in agent_data:
        provider = Provider(
            organization=agent_data["provider"].get("organization", ""),
            url=agent_data["provider"].get("url"),
        )

    # Respect PORT env var for Cloud Run compatibility
    port = int(os.environ.get("PORT", server_data.get("port", 8100)))

    return AgentConfig(
        name=agent_data.get("name", "Unknown Agent"),
        description=agent_data.get("description", ""),
        version=agent_data.get("version", "1.0.0"),
        server=ServerConfig(
            host=server_data.get("host", "0.0.0.0"),
            port=port,
        ),
        llm=LLMConfig(
            provider=llm_data.get("provider", "anthropic"),
            model=llm_data.get("model", "claude-3-5-sonnet-20241022"),
            temperature=llm_data.get("temperature", 0.7),
            max_tokens=llm_data.get("max_tokens", 4096),
        ),
        aex=AEXConfig(
            enabled=aex_data.get("enabled", True),
            gateway_url=os.environ.get("AEX_GATEWAY_URL", aex_data.get("gateway_url", "http://localhost:8080")),
            auto_register=aex_data.get("auto_register", True),
            auto_bid=aex_data.get("auto_bid", True),
            base_rate=aex_data.get("pricing", {}).get("base_rate", 25.0),
            currency=aex_data.get("pricing", {}).get("currency", "USD"),
            ap2_enabled=aex_data.get("ap2_enabled", True),  # AP2 enabled by default
            trust_tier=aex_data.get("trust_tier", "UNVERIFIED"),
            trust_score=aex_data.get("trust_score", 0.3),
        ),
        skills=skills,
        provider=provider,
    )

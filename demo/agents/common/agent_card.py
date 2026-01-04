"""A2A Agent Card models and utilities."""

from dataclasses import dataclass, field, asdict
from typing import Optional
import json


@dataclass
class Provider:
    """Agent provider information."""
    organization: str
    url: Optional[str] = None


@dataclass
class Capabilities:
    """Agent capabilities."""
    streaming: bool = False
    pushNotifications: bool = False
    stateTransitionHistory: bool = False


@dataclass
class Skill:
    """A2A Skill definition."""
    id: str
    name: str
    description: Optional[str] = None
    tags: list[str] = field(default_factory=list)
    examples: list[str] = field(default_factory=list)
    inputModes: list[str] = field(default_factory=lambda: ["text"])
    outputModes: list[str] = field(default_factory=lambda: ["text"])


@dataclass
class AgentCard:
    """A2A Agent Card - describes agent capabilities."""
    name: str
    description: str
    url: str
    version: str
    skills: list[Skill]
    provider: Optional[Provider] = None
    documentationUrl: Optional[str] = None
    capabilities: Capabilities = field(default_factory=Capabilities)
    defaultInputModes: list[str] = field(default_factory=lambda: ["text"])
    defaultOutputModes: list[str] = field(default_factory=lambda: ["text"])
    authentication: Optional[dict] = None

    def to_dict(self) -> dict:
        """Convert to dictionary for JSON serialization."""
        result = {
            "name": self.name,
            "description": self.description,
            "url": self.url,
            "version": self.version,
            "capabilities": asdict(self.capabilities),
            "defaultInputModes": self.defaultInputModes,
            "defaultOutputModes": self.defaultOutputModes,
            "skills": [asdict(s) for s in self.skills],
        }
        if self.provider:
            result["provider"] = asdict(self.provider)
        if self.documentationUrl:
            result["documentationUrl"] = self.documentationUrl
        if self.authentication:
            result["authentication"] = self.authentication
        return result

    def to_json(self) -> str:
        """Convert to JSON string."""
        return json.dumps(self.to_dict(), indent=2)

    @classmethod
    def from_dict(cls, data: dict) -> "AgentCard":
        """Create AgentCard from dictionary."""
        skills = [
            Skill(
                id=s["id"],
                name=s["name"],
                description=s.get("description"),
                tags=s.get("tags", []),
                examples=s.get("examples", []),
                inputModes=s.get("inputModes", ["text"]),
                outputModes=s.get("outputModes", ["text"]),
            )
            for s in data.get("skills", [])
        ]

        provider = None
        if "provider" in data and data["provider"]:
            provider = Provider(
                organization=data["provider"]["organization"],
                url=data["provider"].get("url"),
            )

        capabilities = Capabilities()
        if "capabilities" in data:
            caps = data["capabilities"]
            capabilities = Capabilities(
                streaming=caps.get("streaming", False),
                pushNotifications=caps.get("pushNotifications", False),
                stateTransitionHistory=caps.get("stateTransitionHistory", False),
            )

        return cls(
            name=data["name"],
            description=data["description"],
            url=data["url"],
            version=data["version"],
            skills=skills,
            provider=provider,
            documentationUrl=data.get("documentationUrl"),
            capabilities=capabilities,
            defaultInputModes=data.get("defaultInputModes", ["text"]),
            defaultOutputModes=data.get("defaultOutputModes", ["text"]),
            authentication=data.get("authentication"),
        )

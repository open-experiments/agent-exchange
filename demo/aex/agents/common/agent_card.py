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
class Extension:
    """A2A Extension (e.g., AP2 for payments)."""
    uri: str
    description: Optional[str] = None
    required: bool = False


# AP2 Extension constants
AP2_EXTENSION_URI = "https://github.com/google-agentic-commerce/ap2/v1"


def ap2_extension(required: bool = True) -> Extension:
    """Create an AP2 extension for agent cards."""
    return Extension(
        uri=AP2_EXTENSION_URI,
        description="Supports the Agent Payments Protocol for agent-to-agent payments.",
        required=required,
    )


@dataclass
class Capabilities:
    """Agent capabilities."""
    streaming: bool = False
    pushNotifications: bool = False
    stateTransitionHistory: bool = False
    extensions: list[Extension] = field(default_factory=list)

    def has_ap2_support(self) -> bool:
        """Check if agent supports AP2."""
        return any(ext.uri == AP2_EXTENSION_URI for ext in self.extensions)


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
            extensions = []
            for ext in caps.get("extensions", []):
                extensions.append(Extension(
                    uri=ext["uri"],
                    description=ext.get("description"),
                    required=ext.get("required", False),
                ))
            capabilities = Capabilities(
                streaming=caps.get("streaming", False),
                pushNotifications=caps.get("pushNotifications", False),
                stateTransitionHistory=caps.get("stateTransitionHistory", False),
                extensions=extensions,
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

"""Client for Moltbook.com - Social platform for AI agents.

Moltbook is a social platform where AI agents can:
- Register and get an API key
- Post content and comments
- Discover and follow other agents
- Search for content semantically
"""

import asyncio
import logging
from dataclasses import dataclass, field
from typing import Optional
from datetime import datetime

import aiohttp

logger = logging.getLogger(__name__)

# Default Moltbook URL - can be overridden with MOLTBOOK_BASE_URL env var for local mock
DEFAULT_MOLTBOOK_URL = "https://www.moltbook.com/api/v1"


@dataclass
class MoltbookPost:
    """A post on Moltbook."""
    id: str
    author_id: str
    author_name: str
    title: str
    content: str
    submolt: str  # Community/subreddit
    upvotes: int
    created_at: datetime


@dataclass
class MoltbookAgent:
    """An agent registered on Moltbook."""
    id: str
    name: str
    description: str
    api_key: str = ""
    claim_url: str = ""


@dataclass
class MoltbookClient:
    """Client for interacting with the Moltbook.com API.

    Moltbook is a social platform for AI agents (like Reddit for AI).
    Agents can post, comment, vote, and discover each other.

    API Docs: https://www.moltbook.com/skill.md
    """

    agent_name: str
    agent_description: str = ""
    api_key: str = ""  # Set after registration
    base_url: str = ""  # If empty, uses DEFAULT_MOLTBOOK_URL

    _session: Optional[aiohttp.ClientSession] = field(default=None, init=False)
    _registered: bool = field(default=False, init=False)
    _agent_id: str = field(default="", init=False)

    @property
    def _api_url(self) -> str:
        """Get the API base URL (supports mock server override)."""
        return self.base_url if self.base_url else DEFAULT_MOLTBOOK_URL

    async def _get_session(self) -> aiohttp.ClientSession:
        if self._session is None or self._session.closed:
            self._session = aiohttp.ClientSession()
        return self._session

    def _get_headers(self) -> dict:
        """Get headers with authentication."""
        headers = {"Content-Type": "application/json"}
        if self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"
        return headers

    async def close(self):
        """Close the client session."""
        if self._session and not self._session.closed:
            await self._session.close()

    async def register(self) -> Optional[MoltbookAgent]:
        """Register this agent with Moltbook.

        Returns an agent object with API key and claim URL.
        The claim URL can be used for human verification.
        """
        try:
            session = await self._get_session()
            payload = {
                "name": self.agent_name,
                "description": self.agent_description or f"AI agent: {self.agent_name}",
            }

            async with session.post(
                f"{self._api_url}/agents/register",
                json=payload,
                headers={"Content-Type": "application/json"}
            ) as resp:
                if resp.status in (200, 201):
                    data = await resp.json()
                    self.api_key = data.get("api_key", "")
                    self._agent_id = data.get("agent_id", "")
                    self._registered = True

                    logger.info(f"Registered with Moltbook: {self.agent_name}")
                    logger.info(f"Claim URL: {data.get('claim_url', 'N/A')}")

                    return MoltbookAgent(
                        id=self._agent_id,
                        name=self.agent_name,
                        description=self.agent_description,
                        api_key=self.api_key,
                        claim_url=data.get("claim_url", ""),
                    )
                else:
                    error = await resp.text()
                    logger.error(f"Moltbook registration failed: {error}")
                    return None

        except Exception as e:
            logger.error(f"Error registering with Moltbook: {e}")
            return None

    async def create_post(
        self,
        title: str,
        content: str,
        submolt: str = "agents",  # Default community
        post_type: str = "text"
    ) -> Optional[dict]:
        """Create a new post on Moltbook.

        Args:
            title: Post title
            content: Post content (text or URL)
            submolt: Community to post in (like subreddit)
            post_type: "text" or "link"
        """
        if not self.api_key:
            logger.error("Not registered with Moltbook")
            return None

        try:
            session = await self._get_session()
            payload = {
                "title": title,
                "content": content,
                "submolt": submolt,
                "type": post_type,
            }

            async with session.post(
                f"{self._api_url}/posts",
                json=payload,
                headers=self._get_headers()
            ) as resp:
                if resp.status in (200, 201):
                    data = await resp.json()
                    logger.info(f"Created Moltbook post: {data.get('id')}")
                    return data
                else:
                    error = await resp.text()
                    logger.error(f"Failed to create post: {error}")
                    return None

        except Exception as e:
            logger.error(f"Error creating Moltbook post: {e}")
            return None

    async def comment(
        self,
        post_id: str,
        content: str,
        parent_id: Optional[str] = None
    ) -> Optional[dict]:
        """Add a comment to a post.

        Args:
            post_id: ID of the post to comment on
            content: Comment text
            parent_id: Optional parent comment ID for replies
        """
        if not self.api_key:
            logger.error("Not registered with Moltbook")
            return None

        try:
            session = await self._get_session()
            payload = {"content": content}
            if parent_id:
                payload["parent_id"] = parent_id

            async with session.post(
                f"{self._api_url}/posts/{post_id}/comments",
                json=payload,
                headers=self._get_headers()
            ) as resp:
                if resp.status in (200, 201):
                    data = await resp.json()
                    logger.info(f"Added comment to post {post_id}")
                    return data
                else:
                    error = await resp.text()
                    logger.error(f"Failed to add comment: {error}")
                    return None

        except Exception as e:
            logger.error(f"Error adding comment: {e}")
            return None

    async def search(self, query: str, limit: int = 10) -> list[dict]:
        """Search for content on Moltbook using semantic search.

        Args:
            query: Search query (meaning-based, not just keywords)
            limit: Maximum number of results
        """
        if not self.api_key:
            logger.error("Not registered with Moltbook")
            return []

        try:
            session = await self._get_session()

            async with session.get(
                f"{self._api_url}/search",
                params={"q": query, "limit": limit},
                headers=self._get_headers()
            ) as resp:
                if resp.status == 200:
                    data = await resp.json()
                    return data.get("results", [])
                else:
                    error = await resp.text()
                    logger.error(f"Search failed: {error}")
                    return []

        except Exception as e:
            logger.error(f"Error searching Moltbook: {e}")
            return []

    async def get_feed(self, limit: int = 20) -> list[dict]:
        """Get the agent's personalized feed.

        Returns posts from subscribed submolts and followed agents.
        """
        if not self.api_key:
            logger.error("Not registered with Moltbook")
            return []

        try:
            session = await self._get_session()

            async with session.get(
                f"{self._api_url}/feed",
                params={"limit": limit},
                headers=self._get_headers()
            ) as resp:
                if resp.status == 200:
                    data = await resp.json()
                    return data.get("posts", [])
                else:
                    return []

        except Exception as e:
            logger.error(f"Error getting feed: {e}")
            return []

    async def upvote(self, post_id: str) -> bool:
        """Upvote a post."""
        if not self.api_key:
            return False

        try:
            session = await self._get_session()
            async with session.post(
                f"{self._api_url}/posts/{post_id}/upvote",
                headers=self._get_headers()
            ) as resp:
                return resp.status in (200, 201)
        except Exception:
            return False

    async def downvote(self, post_id: str) -> bool:
        """Downvote a post."""
        if not self.api_key:
            return False

        try:
            session = await self._get_session()
            async with session.post(
                f"{self._api_url}/posts/{post_id}/downvote",
                headers=self._get_headers()
            ) as resp:
                return resp.status in (200, 201)
        except Exception:
            return False

    async def subscribe(self, submolt: str) -> bool:
        """Subscribe to a submolt (community)."""
        if not self.api_key:
            return False

        try:
            session = await self._get_session()
            async with session.post(
                f"{self._api_url}/submolts/{submolt}/subscribe",
                headers=self._get_headers()
            ) as resp:
                return resp.status in (200, 201)
        except Exception:
            return False

    async def heartbeat(self) -> bool:
        """Send a heartbeat to maintain presence.

        Agents should call this every 4+ hours to show they're active.
        """
        try:
            session = await self._get_session()
            async with session.get(
                "https://www.moltbook.com/heartbeat.md",
                headers=self._get_headers()
            ) as resp:
                if resp.status == 200:
                    logger.debug("Moltbook heartbeat sent")
                    return True
                return False
        except Exception:
            return False

    async def broadcast_service(
        self,
        service_name: str,
        service_description: str,
        price: float,
        capabilities: list[str]
    ) -> Optional[dict]:
        """Broadcast that this agent offers a service.

        Creates a post in the 'services' submolt advertising the service.
        Other agents can discover this via search.
        """
        title = f"[SERVICE] {service_name} - {price} AEX"
        content = f"""**{service_name}**

{service_description}

**Price:** {price} AEX tokens
**Capabilities:** {', '.join(capabilities)}
**Agent:** {self.agent_name}

Reply or DM to request this service.
"""
        return await self.create_post(
            title=title,
            content=content,
            submolt="services"
        )

    async def find_services(self, capability: str) -> list[dict]:
        """Find agents offering a specific service capability.

        Searches the 'services' submolt for agents with matching capabilities.
        """
        return await self.search(f"{capability} service agent")

    async def get_agent_profile(self, agent_name: str) -> Optional[dict]:
        """Get another agent's profile including their endpoint.

        Args:
            agent_name: Name of the agent to look up

        Returns:
            Agent profile with endpoint URL for direct A2A communication
        """
        try:
            session = await self._get_session()
            async with session.get(
                f"{self._api_url}/agents/profile",
                params={"name": agent_name},
                headers=self._get_headers()
            ) as resp:
                if resp.status == 200:
                    return await resp.json()
                else:
                    logger.warning(f"Agent profile not found: {agent_name}")
                    return None
        except Exception as e:
            logger.error(f"Error getting agent profile: {e}")
            return None

    async def register_with_endpoint(self, endpoint_url: str) -> Optional[MoltbookAgent]:
        """Register this agent with Moltbook including A2A endpoint.

        Args:
            endpoint_url: HTTP endpoint where this agent can receive A2A requests

        Returns:
            MoltbookAgent with API key for future calls
        """
        try:
            session = await self._get_session()
            payload = {
                "name": self.agent_name,
                "description": self.agent_description or f"AI agent: {self.agent_name}",
                "endpoint": endpoint_url,  # A2A endpoint for direct communication
                "capabilities": [],  # Will be set via broadcast_service
            }

            async with session.post(
                f"{self._api_url}/agents/register",
                json=payload,
                headers={"Content-Type": "application/json"}
            ) as resp:
                if resp.status in (200, 201):
                    data = await resp.json()
                    self.api_key = data.get("api_key", "")
                    self._agent_id = data.get("agent_id", "")
                    self._registered = True

                    logger.info(f"Registered with Moltbook: {self.agent_name} (endpoint: {endpoint_url})")
                    logger.info(f"Claim URL: {data.get('claim_url', 'N/A')}")

                    return MoltbookAgent(
                        id=self._agent_id,
                        name=self.agent_name,
                        description=self.agent_description,
                        api_key=self.api_key,
                        claim_url=data.get("claim_url", ""),
                    )
                else:
                    error = await resp.text()
                    logger.error(f"Moltbook registration failed: {error}")
                    return None

        except Exception as e:
            logger.error(f"Error registering with Moltbook: {e}")
            return None

    async def post_service_request(
        self,
        service_type: str,
        request_details: str,
        budget: float,
        callback_endpoint: str
    ) -> Optional[dict]:
        """Post a service request that providers can respond to.

        Args:
            service_type: Type of service needed (e.g., "research", "writing")
            request_details: Description of what's needed
            budget: Maximum AEX tokens willing to pay
            callback_endpoint: Endpoint where providers should respond

        Returns:
            Post data including post_id for tracking responses
        """
        title = f"[REQUEST] Need {service_type} service - Budget: {budget} AEX"
        content = f"""**Service Request**

**Type:** {service_type}
**Budget:** {budget} AEX tokens
**Details:** {request_details}

**Requesting Agent:** {self.agent_name}
**Callback Endpoint:** {callback_endpoint}

Providers: Please respond via comment with your price and capabilities.
Then call my endpoint to begin the service.
"""
        return await self.create_post(
            title=title,
            content=content,
            submolt="services"
        )

    async def respond_to_request(
        self,
        post_id: str,
        price: float,
        capabilities: list[str],
        endpoint: str
    ) -> Optional[dict]:
        """Respond to a service request as a provider.

        Args:
            post_id: ID of the service request post
            price: Price in AEX tokens
            capabilities: What capabilities this agent offers
            endpoint: Agent's A2A endpoint for direct communication

        Returns:
            Comment data
        """
        content = f"""**Service Offer**

I can fulfill this request!

**Price:** {price} AEX
**Capabilities:** {', '.join(capabilities)}
**Agent:** {self.agent_name}
**Endpoint:** {endpoint}

Call my endpoint to negotiate and begin service.
"""
        return await self.comment(post_id, content)

    async def discover_providers(self, capability: str) -> list[dict]:
        """Discover providers for a capability via Moltbook.

        Searches for service offers and extracts provider endpoints.

        Args:
            capability: Service capability to find (e.g., "research", "writing")

        Returns:
            List of providers with their endpoints and prices
        """
        results = await self.search(f"[SERVICE] {capability}")
        providers = []

        for result in results:
            # Parse service posts to extract provider info
            content = result.get("content", "")
            title = result.get("title", "")

            # Extract price from title (format: "[SERVICE] name - X AEX")
            price = 0.0
            if "AEX" in title:
                try:
                    price_str = title.split("-")[-1].strip().replace("AEX", "").strip()
                    price = float(price_str)
                except (ValueError, IndexError):
                    pass

            # Get agent profile for endpoint
            # Note: author_name is the agent's registered name (e.g., "moltbot-researcher")
            # This is also the agent_id used for Token Bank wallets
            author = result.get("author_name", result.get("author", ""))
            if author:
                profile = await self.get_agent_profile(author)
                if profile:
                    providers.append({
                        "agent_name": author,
                        # Use author (registered name) as agent_id - this matches Token Bank wallet IDs
                        "agent_id": author,
                        "price": price,
                        "endpoint": profile.get("endpoint", ""),
                        "capabilities": profile.get("capabilities", []),
                        "post_id": result.get("id", ""),
                    })

        return providers

    @property
    def is_registered(self) -> bool:
        return self._registered and bool(self.api_key)

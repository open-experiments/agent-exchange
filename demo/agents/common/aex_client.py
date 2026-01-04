"""AEX (Agent Exchange) client for demo agents."""

import asyncio
import logging
from dataclasses import dataclass
from typing import Optional
import aiohttp

logger = logging.getLogger(__name__)


@dataclass
class BidRequest:
    """Incoming bid request from AEX."""
    work_id: str
    domain: str
    requirements: dict
    budget: dict
    deadline_ms: int


@dataclass
class BidResponse:
    """Bid response to submit to AEX."""
    work_id: str
    price: float
    currency: str = "USD"
    confidence: float = 0.9
    estimated_duration_ms: int = 60000
    metadata: Optional[dict] = None


@dataclass
class ContractAward:
    """Contract award notification from AEX."""
    contract_id: str
    work_id: str
    consumer_id: str
    agreed_price: float
    cpa_terms: Optional[dict] = None
    token: str = ""


class AEXClient:
    """Client for interacting with Agent Exchange."""

    def __init__(
        self,
        gateway_url: str,
        provider_id: Optional[str] = None,
        api_key: Optional[str] = None,
        api_secret: Optional[str] = None,
    ):
        self.gateway_url = gateway_url.rstrip("/")
        self.provider_id = provider_id
        self.api_key = api_key
        self.api_secret = api_secret
        self._session: Optional[aiohttp.ClientSession] = None

    async def _get_session(self) -> aiohttp.ClientSession:
        """Get or create HTTP session."""
        if self._session is None or self._session.closed:
            headers = {}
            if self.api_key:
                headers["X-API-Key"] = self.api_key
            if self.api_secret:
                headers["X-API-Secret"] = self.api_secret
            self._session = aiohttp.ClientSession(headers=headers)
        return self._session

    async def close(self):
        """Close the client session."""
        if self._session and not self._session.closed:
            await self._session.close()

    async def register_provider(
        self,
        name: str,
        description: str,
        endpoint: str,
        bid_webhook: str,
        capabilities: list[str],
        contact_email: str = "",
    ) -> dict:
        """Register as a provider with AEX."""
        session = await self._get_session()
        payload = {
            "name": name,
            "description": description,
            "endpoint": endpoint,
            "bid_webhook": bid_webhook,
            "capabilities": capabilities,
            "contact_email": contact_email,
        }

        async with session.post(
            f"{self.gateway_url}/v1/providers",
            json=payload,
        ) as resp:
            if resp.status != 200:
                error = await resp.text()
                raise Exception(f"Failed to register provider: {error}")
            data = await resp.json()
            self.provider_id = data["provider_id"]
            self.api_key = data["api_key"]
            self.api_secret = data["api_secret"]
            logger.info(f"Registered provider: {self.provider_id}")
            return data

    async def subscribe_to_categories(
        self,
        categories: list[str],
        webhook_url: Optional[str] = None,
    ) -> dict:
        """Subscribe to work categories."""
        if not self.provider_id:
            raise ValueError("Provider not registered")

        session = await self._get_session()
        payload = {
            "provider_id": self.provider_id,
            "categories": categories,
            "filters": {},
            "delivery": {
                "method": "webhook" if webhook_url else "polling",
                "webhook_url": webhook_url or "",
            },
        }

        async with session.post(
            f"{self.gateway_url}/v1/subscriptions",
            json=payload,
        ) as resp:
            if resp.status != 200:
                error = await resp.text()
                raise Exception(f"Failed to create subscription: {error}")
            data = await resp.json()
            logger.info(f"Subscribed to categories: {categories}")
            return data

    async def submit_bid(self, bid: BidResponse) -> dict:
        """Submit a bid for work."""
        if not self.provider_id:
            raise ValueError("Provider not registered")

        session = await self._get_session()
        payload = {
            "work_id": bid.work_id,
            "provider_id": self.provider_id,
            "price": bid.price,
            "currency": bid.currency,
            "confidence": bid.confidence,
            "estimated_duration_ms": bid.estimated_duration_ms,
            "metadata": bid.metadata or {},
        }

        async with session.post(
            f"{self.gateway_url}/v1/bids",
            json=payload,
        ) as resp:
            if resp.status != 200:
                error = await resp.text()
                raise Exception(f"Failed to submit bid: {error}")
            data = await resp.json()
            logger.info(f"Submitted bid for work {bid.work_id}")
            return data

    async def report_completion(
        self,
        contract_id: str,
        success: bool,
        metrics: Optional[dict] = None,
        artifacts: Optional[list] = None,
    ) -> dict:
        """Report work completion to AEX for settlement."""
        session = await self._get_session()
        payload = {
            "contract_id": contract_id,
            "success": success,
            "metrics": metrics or {},
            "artifacts": artifacts or [],
        }

        async with session.post(
            f"{self.gateway_url}/v1/contracts/{contract_id}/complete",
            json=payload,
        ) as resp:
            if resp.status != 200:
                error = await resp.text()
                raise Exception(f"Failed to report completion: {error}")
            data = await resp.json()
            logger.info(f"Reported completion for contract {contract_id}")
            return data

    async def get_work_details(self, work_id: str) -> dict:
        """Get details of a work spec."""
        session = await self._get_session()
        async with session.get(f"{self.gateway_url}/v1/work/{work_id}") as resp:
            if resp.status != 200:
                error = await resp.text()
                raise Exception(f"Failed to get work details: {error}")
            return await resp.json()

    async def search_providers(
        self,
        skill_tags: Optional[list[str]] = None,
        domain: Optional[str] = None,
    ) -> list[dict]:
        """Search for providers by skill or domain."""
        session = await self._get_session()
        params = {}
        if skill_tags:
            params["skill_tags"] = ",".join(skill_tags)
        if domain:
            params["domain"] = domain

        async with session.get(
            f"{self.gateway_url}/v1/providers/search",
            params=params,
        ) as resp:
            if resp.status != 200:
                error = await resp.text()
                raise Exception(f"Failed to search providers: {error}")
            data = await resp.json()
            return data.get("providers", [])

    async def submit_work(
        self,
        domain: str,
        requirements: dict,
        budget: dict,
        skill_tags: Optional[list[str]] = None,
        success_criteria: Optional[list] = None,
        bid_window_ms: int = 30000,
    ) -> dict:
        """Submit work as a consumer."""
        session = await self._get_session()
        payload = {
            "domain": domain,
            "requirements": requirements,
            "budget": budget,
            "skill_tags": skill_tags or [],
            "success_criteria": success_criteria or [],
            "bid_window_ms": bid_window_ms,
        }

        async with session.post(
            f"{self.gateway_url}/v1/work",
            json=payload,
        ) as resp:
            if resp.status != 200:
                error = await resp.text()
                raise Exception(f"Failed to submit work: {error}")
            data = await resp.json()
            logger.info(f"Submitted work: {data.get('work_id')}")
            return data

    async def get_evaluation(self, work_id: str) -> dict:
        """Get bid evaluation results."""
        session = await self._get_session()
        async with session.get(
            f"{self.gateway_url}/v1/work/{work_id}/evaluation"
        ) as resp:
            if resp.status != 200:
                error = await resp.text()
                raise Exception(f"Failed to get evaluation: {error}")
            return await resp.json()

    async def award_contract(self, work_id: str, bid_id: str) -> dict:
        """Award contract to winning bid."""
        session = await self._get_session()
        payload = {
            "work_id": work_id,
            "bid_id": bid_id,
        }

        async with session.post(
            f"{self.gateway_url}/v1/contracts",
            json=payload,
        ) as resp:
            if resp.status != 200:
                error = await resp.text()
                raise Exception(f"Failed to award contract: {error}")
            data = await resp.json()
            logger.info(f"Awarded contract: {data.get('contract_id')}")
            return data

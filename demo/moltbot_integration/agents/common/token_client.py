"""Client for interacting with the Token Banking Service."""

import aiohttp
import logging
from dataclasses import dataclass
from typing import Optional

logger = logging.getLogger(__name__)


@dataclass
class Wallet:
    """Represents an agent's wallet."""
    id: str
    agent_id: str
    agent_name: str
    balance: float
    token_type: str


@dataclass
class Transaction:
    """Represents a token transaction."""
    id: str
    from_wallet: str
    to_wallet: str
    amount: float
    token_type: str
    reference: str
    description: str
    status: str


class TokenBankClient:
    """Client for the Token Banking Service."""

    def __init__(self, base_url: str = "http://localhost:8094", auth_token: str = ""):
        self.base_url = base_url.rstrip("/")
        self.auth_token = auth_token  # Secret token for Phase 7 authentication
        self._session: Optional[aiohttp.ClientSession] = None

    async def _get_session(self) -> aiohttp.ClientSession:
        if self._session is None or self._session.closed:
            self._session = aiohttp.ClientSession()
        return self._session

    def _get_headers(self) -> dict:
        """Get headers with authentication if token is set."""
        headers = {"Content-Type": "application/json"}
        if self.auth_token:
            headers["Authorization"] = f"Bearer {self.auth_token}"
        return headers

    async def close(self):
        if self._session and not self._session.closed:
            await self._session.close()

    async def health_check(self) -> bool:
        """Check if the Token Bank is healthy."""
        try:
            session = await self._get_session()
            async with session.get(f"{self.base_url}/health") as resp:
                return resp.status == 200
        except Exception as e:
            logger.error(f"Token Bank health check failed: {e}")
            return False

    async def create_wallet(
        self,
        agent_id: str,
        agent_name: str,
        initial_tokens: float = 0
    ) -> Optional[Wallet]:
        """Create a new wallet for an agent."""
        try:
            session = await self._get_session()
            payload = {
                "agent_id": agent_id,
                "agent_name": agent_name,
                "initial_tokens": initial_tokens,
            }
            async with session.post(f"{self.base_url}/wallets", json=payload) as resp:
                if resp.status in (200, 201):
                    data = await resp.json()
                    return Wallet(
                        id=data["id"],
                        agent_id=data["agent_id"],
                        agent_name=data["agent_name"],
                        balance=data["balance"],
                        token_type=data["token_type"],
                    )
                elif resp.status == 409:
                    logger.info(f"Wallet already exists for {agent_id}")
                    return await self.get_wallet(agent_id)
                else:
                    error = await resp.text()
                    logger.error(f"Failed to create wallet: {error}")
                    return None
        except Exception as e:
            logger.error(f"Error creating wallet: {e}")
            return None

    async def get_wallet(self, agent_id: str) -> Optional[Wallet]:
        """Get wallet information for an agent."""
        try:
            session = await self._get_session()
            async with session.get(
                f"{self.base_url}/wallets/{agent_id}",
                headers=self._get_headers()
            ) as resp:
                if resp.status == 200:
                    data = await resp.json()
                    return Wallet(
                        id=data["id"],
                        agent_id=data["agent_id"],
                        agent_name=data["agent_name"],
                        balance=data["balance"],
                        token_type=data["token_type"],
                    )
                return None
        except Exception as e:
            logger.error(f"Error getting wallet: {e}")
            return None

    async def get_my_wallet(self) -> Optional[Wallet]:
        """Get the authenticated agent's wallet (Phase 7).

        Requires auth_token to be set. Uses the /wallets/me endpoint.
        """
        if not self.auth_token:
            logger.warning("get_my_wallet called without auth_token")
            return None

        try:
            session = await self._get_session()
            async with session.get(
                f"{self.base_url}/wallets/me",
                headers=self._get_headers()
            ) as resp:
                if resp.status == 200:
                    data = await resp.json()
                    return Wallet(
                        id=data["id"],
                        agent_id=data["agent_id"],
                        agent_name=data["agent_name"],
                        balance=data["balance"],
                        token_type=data["token_type"],
                    )
                elif resp.status == 401:
                    logger.error("Authentication failed for get_my_wallet")
                    return None
                else:
                    error = await resp.text()
                    logger.error(f"get_my_wallet failed: {error}")
                    return None
        except Exception as e:
            logger.error(f"Error getting my wallet: {e}")
            return None

    async def get_balance(self, agent_id: str) -> Optional[float]:
        """Get the token balance for an agent."""
        try:
            session = await self._get_session()
            async with session.get(f"{self.base_url}/wallets/{agent_id}/balance") as resp:
                if resp.status == 200:
                    data = await resp.json()
                    return data["balance"]
                return None
        except Exception as e:
            logger.error(f"Error getting balance: {e}")
            return None

    async def deposit(
        self,
        agent_id: str,
        amount: float,
        description: str = ""
    ) -> Optional[Transaction]:
        """Deposit tokens into an agent's wallet."""
        try:
            session = await self._get_session()
            payload = {"amount": amount, "description": description}
            async with session.post(
                f"{self.base_url}/wallets/{agent_id}/deposit",
                json=payload
            ) as resp:
                if resp.status == 200:
                    data = await resp.json()
                    return Transaction(
                        id=data["id"],
                        from_wallet=data["from_wallet"],
                        to_wallet=data["to_wallet"],
                        amount=data["amount"],
                        token_type=data["token_type"],
                        reference=data["reference"],
                        description=data["description"],
                        status=data["status"],
                    )
                return None
        except Exception as e:
            logger.error(f"Error depositing: {e}")
            return None

    async def withdraw(
        self,
        agent_id: str,
        amount: float,
        description: str = ""
    ) -> Optional[Transaction]:
        """Withdraw tokens from an agent's wallet."""
        try:
            session = await self._get_session()
            payload = {"amount": amount, "description": description}
            async with session.post(
                f"{self.base_url}/wallets/{agent_id}/withdraw",
                json=payload
            ) as resp:
                if resp.status == 200:
                    data = await resp.json()
                    return Transaction(
                        id=data["id"],
                        from_wallet=data["from_wallet"],
                        to_wallet=data["to_wallet"],
                        amount=data["amount"],
                        token_type=data["token_type"],
                        reference=data["reference"],
                        description=data["description"],
                        status=data["status"],
                    )
                return None
        except Exception as e:
            logger.error(f"Error withdrawing: {e}")
            return None

    async def transfer(
        self,
        from_agent_id: str,
        to_agent_id: str,
        amount: float,
        reference: str = "",
        description: str = ""
    ) -> Optional[Transaction]:
        """Transfer tokens between agents."""
        try:
            session = await self._get_session()
            payload = {
                "from_agent_id": from_agent_id,
                "to_agent_id": to_agent_id,
                "amount": amount,
                "reference": reference,
                "description": description,
            }
            async with session.post(f"{self.base_url}/transfers", json=payload) as resp:
                if resp.status == 200:
                    data = await resp.json()
                    return Transaction(
                        id=data["id"],
                        from_wallet=data["from_wallet"],
                        to_wallet=data["to_wallet"],
                        amount=data["amount"],
                        token_type=data["token_type"],
                        reference=data["reference"],
                        description=data["description"],
                        status=data["status"],
                    )
                else:
                    error = await resp.text()
                    logger.error(f"Transfer failed: {error}")
                    return None
        except Exception as e:
            logger.error(f"Error transferring: {e}")
            return None

    async def get_transaction_history(self, agent_id: str) -> list[Transaction]:
        """Get transaction history for an agent."""
        try:
            session = await self._get_session()
            async with session.get(f"{self.base_url}/wallets/{agent_id}/history") as resp:
                if resp.status == 200:
                    data = await resp.json()
                    return [
                        Transaction(
                            id=tx["id"],
                            from_wallet=tx["from_wallet"],
                            to_wallet=tx["to_wallet"],
                            amount=tx["amount"],
                            token_type=tx["token_type"],
                            reference=tx["reference"],
                            description=tx["description"],
                            status=tx["status"],
                        )
                        for tx in data.get("transactions", [])
                    ]
                return []
        except Exception as e:
            logger.error(f"Error getting history: {e}")
            return []

    async def get_all_wallets(self) -> list[Wallet]:
        """Get all wallets."""
        try:
            session = await self._get_session()
            async with session.get(f"{self.base_url}/wallets") as resp:
                if resp.status == 200:
                    data = await resp.json()
                    return [
                        Wallet(
                            id=w["id"],
                            agent_id=w["agent_id"],
                            agent_name=w["agent_name"],
                            balance=w["balance"],
                            token_type=w["token_type"],
                        )
                        for w in data.get("wallets", [])
                    ]
                return []
        except Exception as e:
            logger.error(f"Error getting wallets: {e}")
            return []

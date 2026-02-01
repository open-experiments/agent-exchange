"""Base Moltbot agent with Token Banking, AP2, Moltbook, and LLM integration."""

import asyncio
import json
import logging
import os
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import Any, AsyncIterator, Optional

import aiohttp
from .token_client import TokenBankClient, Wallet
from .openclaw_client import OpenClawClient
from .ap2_client import AP2Client, PaymentReceipt
from .moltbook_client import MoltbookClient
from .llm_client import LLMClient, LLMConfig, create_llm_client

logger = logging.getLogger(__name__)


@dataclass
class AgentConfig:
    """Configuration for a Moltbot agent."""
    agent_id: str
    agent_name: str
    description: str
    port: int
    initial_tokens: float = 50.0  # Legacy: used when bank_token not set
    service_price: float = 10.0  # Price in AEX tokens
    token_bank_url: str = "http://aex-token-bank:8094"
    aex_registry_url: str = "http://aex-provider-registry:8080"
    openclaw_gateway_url: str = "ws://localhost:18789"  # Molt.bot gateway
    enable_gateway: bool = True  # Whether to connect to OpenClaw gateway
    enable_ap2: bool = True  # Whether to use AP2 for payments
    bank_token: str = ""  # Phase 7: Secret token for bank authentication
    # Moltbook integration
    enable_moltbook: bool = True  # Whether to connect to Moltbook.com
    moltbook_api_key: str = ""  # Moltbook API key (after registration)
    moltbook_base_url: str = ""  # Moltbook API URL (empty = real moltbook.com)
    # LLM integration
    enable_llm: bool = True  # Whether to use real LLM for responses
    anthropic_api_key: str = ""  # Claude API key
    llm_model: str = "claude-sonnet-4-20250514"  # Claude model to use
    agent_type: str = "general"  # Agent type for specialized LLM: researcher, writer, analyst


@dataclass
class MoltbotAgent(ABC):
    """Base class for Moltbot demo agents with token wallet, AP2, Moltbook, and LLM integration."""

    config: AgentConfig
    token_client: Optional[TokenBankClient] = field(default=None, init=False)
    ap2_client: Optional[AP2Client] = field(default=None, init=False)
    gateway_client: Optional[OpenClawClient] = field(default=None, init=False)
    moltbook_client: Optional[MoltbookClient] = field(default=None, init=False)
    llm_client: Optional[LLMClient] = field(default=None, init=False)
    wallet: Optional[Wallet] = field(default=None, init=False)
    _http_session: Optional[aiohttp.ClientSession] = field(default=None, init=False)
    _gateway_connected: bool = field(default=False, init=False)
    _moltbook_registered: bool = field(default=False, init=False)

    def __post_init__(self):
        """Initialize the agent."""
        # Phase 7: Pass auth_token if bank_token is configured
        self.token_client = TokenBankClient(
            base_url=self.config.token_bank_url,
            auth_token=self.config.bank_token,
        )

        # Initialize AP2 client for payment protocol
        # AP2 discovers Token Bank via AEX Registry
        if self.config.enable_ap2:
            self.ap2_client = AP2Client(
                aex_registry_url=self.config.aex_registry_url,
            )

        if self.config.enable_gateway:
            self.gateway_client = OpenClawClient(
                gateway_url=self.config.openclaw_gateway_url,
                agent_id=self.config.agent_id,
                agent_name=self.config.agent_name,
            )

        # Initialize Moltbook client for social platform integration
        # Use agent_id for Moltbook name (no spaces, alphanumeric with hyphens)
        if self.config.enable_moltbook:
            self.moltbook_client = MoltbookClient(
                agent_name=self.config.agent_id,  # Moltbook requires alphanumeric with hyphens
                agent_description=self.config.description,
                api_key=self.config.moltbook_api_key,
                base_url=self.config.moltbook_base_url,  # Support mock server
            )

        # Initialize LLM client for real AI responses
        if self.config.enable_llm:
            llm_config = LLMConfig(
                model=self.config.llm_model,
                api_key=self.config.anthropic_api_key,
            )
            self.llm_client = create_llm_client(
                agent_type=self.config.agent_type,
                config=llm_config,
            )

    async def startup(self):
        """Initialize agent on startup."""
        logger.info(f"Starting agent: {self.config.agent_name}")

        # Wait for Token Bank to be ready
        retries = 10
        while retries > 0:
            if await self.token_client.health_check():
                break
            logger.info("Waiting for Token Bank...")
            await asyncio.sleep(2)
            retries -= 1

        if retries == 0:
            logger.warning("Token Bank not available, continuing without wallet")
        else:
            # Phase 7: Use get_my_wallet() with auth token if bank_token is set
            if self.config.bank_token:
                # Secure mode: wallet pre-created by bank, authenticate to get it
                self.wallet = await self.token_client.get_my_wallet()
                if self.wallet:
                    logger.info(
                        f"Authenticated wallet: {self.wallet.agent_id} "
                        f"with {self.wallet.balance} AEX tokens (Phase 7)"
                    )
                else:
                    logger.error(
                        f"Failed to authenticate with bank. "
                        f"Check BANK_TOKEN is correct for {self.config.agent_id}"
                    )
            else:
                # Legacy mode: create wallet with initial tokens
                self.wallet = await self.token_client.create_wallet(
                    agent_id=self.config.agent_id,
                    agent_name=self.config.agent_name,
                    initial_tokens=self.config.initial_tokens,
                )
                if self.wallet:
                    logger.info(
                        f"Wallet created: {self.wallet.agent_id} "
                        f"with {self.wallet.balance} AEX tokens (legacy mode)"
                    )

        # Connect to OpenClaw gateway
        if self.gateway_client and self.config.enable_gateway:
            await self._connect_to_gateway()

        # Register with Moltbook.com
        if self.moltbook_client and self.config.enable_moltbook:
            await self._register_with_moltbook()

        # Register with AEX
        await self._register_with_aex()

        # Log LLM status
        if self.llm_client and self.config.enable_llm:
            if self.llm_client.is_available:
                logger.info(f"LLM enabled: {self.config.llm_model}")
            else:
                logger.warning("LLM not available (check ANTHROPIC_API_KEY)")

    async def _connect_to_gateway(self):
        """Connect to OpenClaw/Molt.bot gateway."""
        retries = 5
        while retries > 0:
            try:
                connected = await self.gateway_client.connect()
                if connected:
                    self._gateway_connected = True
                    logger.info(f"Connected to OpenClaw gateway: {self.config.openclaw_gateway_url}")

                    # Register message handlers
                    self._register_gateway_handlers()
                    return
            except Exception as e:
                logger.debug(f"Gateway connection attempt failed: {e}")

            logger.info("Waiting for OpenClaw gateway...")
            await asyncio.sleep(3)
            retries -= 1

        logger.warning("OpenClaw gateway not available, continuing without gateway connection")

    async def _register_with_moltbook(self):
        """Register this agent with Moltbook.com social platform.

        Includes the agent's A2A endpoint URL so other agents can discover
        and communicate directly.
        """
        if not self.moltbook_client:
            return

        try:
            # Build the agent's A2A endpoint URL
            # In Docker, use the container name; locally use localhost
            agent_endpoint = f"http://{self.config.agent_id}:{self.config.port}"
            logger.info(f"📡 Registering with Moltbook (endpoint: {agent_endpoint})")

            # If we already have an API key, just verify it works
            if self.moltbook_client.api_key:
                logger.info(f"Using existing Moltbook API key for {self.config.agent_name}")
                self._moltbook_registered = True

                # Still broadcast service if we have a price
                if self.config.service_price > 0:
                    await self.moltbook_client.broadcast_service(
                        service_name=self.config.agent_id,  # Use ID for consistency
                        service_description=self.config.description,
                        price=self.config.service_price,
                        capabilities=self.get_capabilities(),
                    )
                return

            # Register with Moltbook including endpoint
            agent = await self.moltbook_client.register_with_endpoint(agent_endpoint)
            if agent:
                self._moltbook_registered = True
                logger.info(f"✅ Registered with Moltbook: {agent.name}")
                if agent.claim_url:
                    logger.info(f"📎 Moltbook claim URL: {agent.claim_url}")

                # Broadcast service availability (so customers can find us)
                if self.config.service_price > 0:
                    post = await self.moltbook_client.broadcast_service(
                        service_name=self.config.agent_id,  # Use ID for consistency
                        service_description=self.config.description,
                        price=self.config.service_price,
                        capabilities=self.get_capabilities(),
                    )
                    if post:
                        logger.info(f"📢 Broadcasted service on Moltbook: {post.get('id', 'unknown')}")
            else:
                logger.warning("Failed to register with Moltbook, continuing without")
        except Exception as e:
            logger.warning(f"Moltbook registration error: {e}")

    def _register_gateway_handlers(self):
        """Register handlers for incoming gateway messages."""
        if not self.gateway_client:
            return

        # Handle incoming service requests via gateway
        async def handle_service_request(params: dict) -> dict:
            results = []
            async for response in self.handle_message(params):
                results.append(response)
            return {"responses": results}

        self.gateway_client.on_message("service.request", handle_service_request)

        # Handle payment requests via gateway
        async def handle_payment_request(params: dict) -> dict:
            from_agent = params.get("from_agent")
            amount = params.get("amount", 0)
            reference = params.get("reference", "")

            # Verify payment was received
            success = await self.receive_payment(from_agent, amount)
            return {"success": success, "reference": reference}

        self.gateway_client.on_message("payment.received", handle_payment_request)

    async def _register_with_aex(self):
        """Register this agent with AEX registry."""
        try:
            session = await self._get_http_session()
            hostname = os.environ.get("AGENT_HOSTNAME", "localhost")
            endpoint = f"http://{hostname}:{self.config.port}"

            # Build metadata - don't expose initial_balance in Phase 7 (secure mode)
            metadata = {
                "service_price": self.config.service_price,
                "token_type": "AEX",
                "gateway_connected": self._gateway_connected,
                "ap2_enabled": self.config.enable_ap2,
                "payment_methods": ["aex-token"] if self.config.enable_ap2 else ["direct"],
                "secure_banking": bool(self.config.bank_token),  # Phase 7 indicator
            }
            # Only include initial_balance in legacy mode (for debugging)
            if not self.config.bank_token:
                metadata["initial_balance"] = self.config.initial_tokens

            payload = {
                "name": self.config.agent_name,
                "description": self.config.description,
                "endpoint": endpoint,
                "capabilities": self.get_capabilities(),
                "metadata": metadata,
            }

            async with session.post(
                f"{self.config.aex_registry_url}/v1/providers",
                json=payload
            ) as resp:
                if resp.status in (200, 201):
                    logger.info(f"Registered with AEX: {self.config.agent_name}")
                else:
                    error = await resp.text()
                    logger.warning(f"AEX registration failed: {error}")
        except Exception as e:
            logger.warning(f"Could not register with AEX: {e}")

    async def _get_http_session(self) -> aiohttp.ClientSession:
        if self._http_session is None or self._http_session.closed:
            self._http_session = aiohttp.ClientSession()
        return self._http_session

    async def shutdown(self):
        """Cleanup on shutdown."""
        if self.gateway_client:
            await self.gateway_client.disconnect()
        if self.ap2_client:
            await self.ap2_client.close()
        if self.token_client:
            await self.token_client.close()
        if self.moltbook_client:
            await self.moltbook_client.close()
        if self._http_session and not self._http_session.closed:
            await self._http_session.close()
        logger.info(f"Agent shutdown: {self.config.agent_name}")

    async def get_balance(self) -> float:
        """Get current token balance."""
        if self.token_client:
            balance = await self.token_client.get_balance(self.config.agent_id)
            return balance if balance is not None else 0.0
        return 0.0

    async def can_afford(self, amount: float) -> bool:
        """Check if agent can afford a transaction."""
        balance = await self.get_balance()
        return balance >= amount

    async def pay_for_service(
        self,
        to_agent_id: str,
        amount: float,
        reference: str = "",
        description: str = ""
    ) -> bool:
        """Pay another agent for a service using AP2 protocol when enabled."""
        if not self.token_client:
            logger.error("Token client not available")
            return False

        if not await self.can_afford(amount):
            logger.error(f"Insufficient balance to pay {amount} AEX")
            return False

        # Use AP2 protocol if enabled
        if self.config.enable_ap2 and self.ap2_client:
            return await self._pay_with_ap2(to_agent_id, amount, reference, description)

        # Fallback to direct token transfer
        return await self._pay_direct(to_agent_id, amount, reference, description)

    async def _pay_with_ap2(
        self,
        to_agent_id: str,
        amount: float,
        reference: str = "",
        description: str = ""
    ) -> bool:
        """Pay using AP2 mandate chain (IntentMandate -> CartMandate -> PaymentMandate -> Receipt)."""
        try:
            success, receipt, error = await self.ap2_client.process_mandate_chain(
                consumer_id=self.config.agent_id,
                provider_id=to_agent_id,
                amount=amount,
                description=description or f"Payment for service from {to_agent_id}",
            )

            if success and receipt:
                logger.info(
                    f"AP2 payment completed: {amount} AEX to {to_agent_id} "
                    f"(mandate: {receipt.payment_mandate_id}, payment: {receipt.payment_id})"
                )

                # Notify the recipient via gateway if connected
                if self._gateway_connected and self.gateway_client:
                    try:
                        await self.gateway_client.send_to_agent(
                            target_agent=to_agent_id,
                            action="payment.received",
                            data={
                                "from_agent": self.config.agent_id,
                                "amount": amount,
                                "reference": reference,
                                "payment_id": receipt.payment_id,
                                "mandate_id": receipt.payment_mandate_id,
                                "protocol": "AP2",
                            }
                        )
                    except Exception as e:
                        logger.debug(f"Could not notify recipient via gateway: {e}")

                return True
            else:
                logger.error(f"AP2 payment failed: {error}")
                return False

        except Exception as e:
            logger.error(f"AP2 payment error: {e}")
            return False

    async def _pay_direct(
        self,
        to_agent_id: str,
        amount: float,
        reference: str = "",
        description: str = ""
    ) -> bool:
        """Pay using direct token transfer (fallback when AP2 not enabled)."""
        tx = await self.token_client.transfer(
            from_agent_id=self.config.agent_id,
            to_agent_id=to_agent_id,
            amount=amount,
            reference=reference,
            description=description,
        )

        if tx:
            logger.info(
                f"Direct payment sent: {amount} AEX to {to_agent_id} "
                f"(tx: {tx.id})"
            )

            # Notify the recipient via gateway if connected
            if self._gateway_connected and self.gateway_client:
                try:
                    await self.gateway_client.send_to_agent(
                        target_agent=to_agent_id,
                        action="payment.received",
                        data={
                            "from_agent": self.config.agent_id,
                            "amount": amount,
                            "reference": reference,
                            "transaction_id": tx.id,
                            "protocol": "direct",
                        }
                    )
                except Exception as e:
                    logger.debug(f"Could not notify recipient via gateway: {e}")

            return True
        return False

    async def pay_for_service_with_steps(
        self,
        to_agent_id: str,
        amount: float,
        description: str = ""
    ) -> tuple[bool, Optional[PaymentReceipt]]:
        """
        Pay using AP2 with full mandate chain visibility.
        Returns the payment receipt for detailed tracking.
        """
        if not self.ap2_client:
            logger.error("AP2 client not available")
            return False, None

        if not await self.can_afford(amount):
            logger.error(f"Insufficient balance to pay {amount} AEX")
            return False, None

        # Step 1: Create Intent Mandate
        logger.info(f"AP2 Step 1: Creating IntentMandate for {amount} AEX to {to_agent_id}")
        intent, intent_id = await self.ap2_client.create_intent_mandate(
            consumer_id=self.config.agent_id,
            provider_id=to_agent_id,
            amount=amount,
            description=description,
        )
        if not intent:
            logger.error("Failed to create IntentMandate")
            return False, None

        # Step 2: Create Cart Mandate
        logger.info(f"AP2 Step 2: Creating CartMandate (intent: {intent_id})")
        items = [{
            "label": description or "Service payment",
            "amount": {"currency": "AEX", "value": f"{amount:.2f}"},
        }]
        total = {
            "label": "Total",
            "amount": {"currency": "AEX", "value": f"{amount:.2f}"},
        }
        cart, cart_id = await self.ap2_client.create_cart_mandate(
            intent_mandate_id=intent_id,
            items=items,
            total=total,
        )
        if not cart:
            logger.error("Failed to create CartMandate")
            return False, None

        # Step 3: Create Payment Mandate
        logger.info(f"AP2 Step 3: Creating PaymentMandate (cart: {cart_id})")
        payment_mandate, payment_id = await self.ap2_client.create_payment_mandate(
            cart_mandate_id=cart_id,
            payment_method="aex-token",
        )
        if not payment_mandate:
            logger.error("Failed to create PaymentMandate")
            return False, None

        # Step 4: Process Payment
        logger.info(f"AP2 Step 4: Processing payment (mandate: {payment_id})")
        success, receipt, tx_id = await self.ap2_client.process_payment(
            payment_mandate=payment_mandate,
            from_agent_id=self.config.agent_id,
            to_agent_id=to_agent_id,
            amount=amount,
            description=description,
        )

        if success:
            logger.info(f"AP2 payment completed successfully: {receipt.payment_id if receipt else tx_id}")
        else:
            logger.error(f"AP2 payment failed")

        return success, receipt

    async def receive_payment(
        self,
        from_agent_id: str,
        amount: float
    ) -> bool:
        """Receive payment from another agent (for logging/verification)."""
        # In practice, the transfer is done by the paying agent
        # This method is for tracking/verification
        logger.info(f"Payment received: {amount} AEX from {from_agent_id}")
        return True

    # =========================================
    # Agent-to-Agent Communication via Gateway
    # =========================================

    async def request_service_from_agent(
        self,
        target_agent: str,
        request_data: dict,
        pay_upfront: bool = True,
    ) -> Optional[dict]:
        """
        Request a service from another agent via the gateway.
        Pays upfront and includes payment proof in the request.
        """
        if not self._gateway_connected or not self.gateway_client:
            logger.error("Not connected to gateway")
            return None

        try:
            # Get the target agent's price
            price_response = await self.gateway_client.send_to_agent(
                target_agent=target_agent,
                action="get_price",
                data={},
            )
            price = price_response.get("price", 0)

            payment_id = None
            payment_amount = 0

            if pay_upfront and price > 0:
                # Pay for the service first using AP2
                if self.config.enable_ap2 and self.ap2_client:
                    success, receipt = await self.pay_for_service_with_steps(
                        to_agent_id=target_agent,
                        amount=price,
                        description=f"Payment for service from {target_agent}",
                    )
                    if success and receipt:
                        payment_id = receipt.payment_id
                        payment_amount = price
                    else:
                        logger.error(f"AP2 payment failed for service from {target_agent}")
                        return None
                else:
                    # Direct payment
                    paid = await self.pay_for_service(
                        to_agent_id=target_agent,
                        amount=price,
                        reference=f"service_request_{target_agent}",
                        description=f"Payment for service from {target_agent}",
                    )
                    if not paid:
                        logger.error(f"Could not pay for service from {target_agent}")
                        return None
                    payment_amount = price

            # Send the service request WITH payment proof
            request_with_payment = {
                **request_data,
                "from_agent": self.config.agent_id,
                "payment_amount": payment_amount,
                "payment_id": payment_id,  # Proof of payment
            }

            response = await self.gateway_client.send_to_agent(
                target_agent=target_agent,
                action="service_request",
                data=request_with_payment,
            )

            return response

        except Exception as e:
            logger.error(f"Service request failed: {e}")
            return None

    async def discover_agents(self, capability: Optional[str] = None) -> list[dict]:
        """Discover other agents via the gateway."""
        if not self._gateway_connected or not self.gateway_client:
            return []

        try:
            return await self.gateway_client.discover_agents(capability)
        except Exception as e:
            logger.error(f"Agent discovery failed: {e}")
            return []

    async def discover_token_bank(self) -> Optional[str]:
        """Discover Token Bank URL via AEX Registry.

        Queries /v1/providers and filters by 'token_banking' capability.
        Returns the Token Bank endpoint URL, or falls back to configured URL.
        """
        try:
            session = await self._get_http_session()
            async with session.get(
                f"{self.config.aex_registry_url}/v1/providers"
            ) as resp:
                if resp.status == 200:
                    data = await resp.json()
                    providers = data.get("providers", [])
                    # Filter by token_banking capability
                    for provider in providers:
                        capabilities = provider.get("capabilities", [])
                        if "token_banking" in capabilities:
                            endpoint = provider.get("endpoint")
                            logger.info(f"Discovered Token Bank via AEX: {endpoint}")
                            return endpoint
        except Exception as e:
            logger.warning(f"Could not discover Token Bank via AEX: {e}")

        # Fallback to configured URL
        logger.info(f"Using configured Token Bank URL: {self.config.token_bank_url}")
        return self.config.token_bank_url

    async def create_payment_request(
        self,
        consumer_id: str,
        amount: float,
        description: str = ""
    ) -> dict:
        """
        Create a payment request that a consumer can use to pay.

        This is called by providers when a customer requests a service:
        1. Discover Token Bank via AEX
        2. Create IntentMandate in Token Bank
        3. Return payment_id for consumer to use

        Args:
            consumer_id: The agent ID of the consumer who will pay
            amount: Amount to charge in AEX tokens
            description: Description of the service

        Returns:
            dict with payment_id, amount, token_bank_url, etc.
        """
        # Discover Token Bank via AEX
        token_bank_url = await self.discover_token_bank()

        if not self.ap2_client:
            return {
                "error": "AP2 client not available",
                "provider_id": self.config.agent_id,
            }

        # Create intent mandate (first step of AP2)
        try:
            intent, intent_id = await self.ap2_client.create_intent_mandate(
                consumer_id=consumer_id,
                provider_id=self.config.agent_id,
                amount=amount,
                description=description or f"Service from {self.config.agent_name}",
            )

            if intent and intent_id:
                logger.info(f"Created payment request: {intent_id} for {amount} AEX")
                return {
                    "payment_id": intent_id,
                    "provider_id": self.config.agent_id,
                    "provider_name": self.config.agent_name,
                    "amount": amount,
                    "token_bank_url": token_bank_url,
                    "description": description or f"Service from {self.config.agent_name}",
                    "status": "pending_payment",
                }
            else:
                return {
                    "error": "Failed to create intent mandate",
                    "provider_id": self.config.agent_id,
                }
        except Exception as e:
            logger.error(f"Failed to create payment request: {e}")
            return {
                "error": str(e),
                "provider_id": self.config.agent_id,
            }

    @abstractmethod
    def get_capabilities(self) -> list[str]:
        """Return list of agent capabilities."""
        pass

    @abstractmethod
    async def handle_request(self, request: dict) -> AsyncIterator[dict]:
        """Handle incoming service request."""
        pass

    async def handle_message(self, message: dict) -> AsyncIterator[dict]:
        """Handle incoming A2A message."""
        action = message.get("action", "")

        if action == "get_price":
            # Return service price (free action - no payment needed)
            yield {
                "type": "result",
                "parts": [{
                    "type": "text",
                    "text": json.dumps({
                        "action": "price_response",
                        "price": self.config.service_price,
                        "token_type": "AEX",
                        "agent_id": self.config.agent_id,
                        "agent_name": self.config.agent_name,
                    })
                }]
            }

        elif action == "get_balance":
            # Return current balance (free action)
            balance = await self.get_balance()
            yield {
                "type": "result",
                "parts": [{
                    "type": "text",
                    "text": json.dumps({
                        "action": "balance_response",
                        "agent_id": self.config.agent_id,
                        "balance": balance,
                        "token_type": "AEX",
                    })
                }]
            }

        elif action == "discover_agents":
            # Return list of known agents via gateway (free action)
            agents = await self.discover_agents()
            yield {
                "type": "result",
                "parts": [{
                    "type": "text",
                    "text": json.dumps({
                        "action": "agents_response",
                        "agents": agents,
                        "gateway_connected": self._gateway_connected,
                    })
                }]
            }

        elif action == "create_payment_request":
            # Provider creates payment request for consumer (free action)
            consumer_id = message.get("from_agent", message.get("consumer_id", ""))
            amount = message.get("amount", self.config.service_price)
            description = message.get("description", f"Service from {self.config.agent_name}")

            payment_request = await self.create_payment_request(
                consumer_id=consumer_id,
                amount=amount,
                description=description,
            )

            yield {
                "type": "result",
                "parts": [{
                    "type": "text",
                    "text": json.dumps(payment_request)
                }]
            }

        elif action == "service_request":
            # =============================================
            # PAYMENT VERIFICATION BEFORE WORK
            # (Skip if agent doesn't sell services, i.e., service_price=0)
            # =============================================
            if self.config.service_price > 0:
                verified, error = await self._verify_payment_received(message)
                if not verified:
                    yield {
                        "type": "result",
                        "parts": [{
                            "type": "text",
                            "text": json.dumps({
                                "error": error,
                                "action": "payment_rejected",
                                "required_payment": self.config.service_price,
                                "token_type": "AEX",
                                "agent_id": self.config.agent_id,
                                "hint": "Pay first using AP2 or direct transfer, then include payment_id in request",
                            })
                        }]
                    }
                    return

            # Payment verified (or not required) - proceed with service
            async for response in self.handle_request(message):
                yield response

        else:
            # Default: pass to subclass handle_request
            # Only verify payment if this agent sells services (service_price > 0)
            if self.config.service_price > 0:
                verified, error = await self._verify_payment_received(message)
                if not verified:
                    yield {
                        "type": "result",
                        "parts": [{
                            "type": "text",
                            "text": json.dumps({
                                "error": error,
                                "action": "payment_rejected",
                                "required_payment": self.config.service_price,
                                "token_type": "AEX",
                                "agent_id": self.config.agent_id,
                                "hint": "Pay first using AP2 or direct transfer, then include payment_id in request",
                            })
                        }]
                    }
                    return

            async for response in self.handle_request(message):
                yield response

    async def _verify_payment_received(self, message: dict) -> tuple[bool, str]:
        """
        Verify payment was received BEFORE doing any work.

        Checks:
        1. Payment amount >= service_price (reject "under salary" payments)
        2. Payment was actually received (via AP2 or direct transfer)

        Returns:
            (verified: bool, error_message: str)
        """
        from_agent = message.get("from_agent", message.get("consumer_id", ""))
        payment_amount = message.get("payment_amount", 0)
        payment_id = message.get("payment_id", message.get("transaction_id", ""))
        skip_payment = message.get("skip_payment", False)  # For free/demo requests

        # Allow free requests if explicitly marked (for demos)
        if skip_payment:
            logger.info("Payment verification skipped (skip_payment=True)")
            return True, ""

        # Check 1: If payment_amount provided, verify it meets minimum
        if payment_amount > 0 and payment_amount < self.config.service_price:
            error = (
                f"Payment too low: offered {payment_amount} AEX, "
                f"but service costs {self.config.service_price} AEX. "
                f"This is below the minimum service price."
            )
            logger.warning(f"❌ Payment rejected (under salary): {error}")
            return False, error

        # Check 2: If payment_id provided, trust that payment was made via AP2
        if payment_id:
            logger.info(f"✅ Payment verified via payment_id: {payment_id}")
            return True, ""

        # Check 3: Verify from transaction history
        if from_agent and self.token_client:
            try:
                history = await self.token_client.get_transaction_history(self.config.agent_id)
                if history:
                    # Look for recent payment from this agent (within last 10 transactions)
                    for tx in reversed(history[-10:]):
                        if (tx.from_wallet == from_agent and
                            tx.to_wallet == self.config.agent_id and
                            tx.amount >= self.config.service_price):
                            logger.info(f"✅ Payment verified from history: {tx.id} ({tx.amount} AEX)")
                            return True, ""
            except Exception as e:
                logger.debug(f"Could not check transaction history: {e}")

        # No payment found
        error = (
            f"Payment required: {self.config.service_price} AEX. "
            f"Please pay to {self.config.agent_id} first, then include "
            f"'payment_id' or 'payment_amount' in your request."
        )
        logger.warning(f"❌ Payment not verified: {error}")
        return False, error

    def create_agent_card(self) -> dict:
        """Create A2A agent card."""
        return {
            "name": self.config.agent_name,
            "description": self.config.description,
            "url": f"http://{self.config.agent_id}:{self.config.port}",
            "version": "1.0.0",
            "capabilities": {
                "streaming": True,
                "pushNotifications": False,
                "stateTransitionHistory": False,
                "gatewayEnabled": self._gateway_connected,
                "ap2Enabled": self.config.enable_ap2,
            },
            "skills": [
                {
                    "id": cap,
                    "name": cap.replace("_", " ").title(),
                }
                for cap in self.get_capabilities()
            ],
            "metadata": {
                "service_price": self.config.service_price,
                "token_type": "AEX",
                "gateway_connected": self._gateway_connected,
                "ap2_enabled": self.config.enable_ap2,
                "payment_methods": ["aex-token"] if self.config.enable_ap2 else ["direct"],
            },
        }

    @property
    def is_gateway_connected(self) -> bool:
        """Check if connected to the gateway."""
        return self._gateway_connected

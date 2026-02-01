"""AP2 (Agent Payments Protocol) client for Moltbot agents."""

import asyncio
import logging
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from typing import Any, Optional
from uuid import uuid4

import aiohttp

logger = logging.getLogger(__name__)


@dataclass
class Amount:
    """Currency amount."""
    currency: str
    value: str


@dataclass
class PaymentItem:
    """Item in a payment request."""
    label: str
    amount: Amount


@dataclass
class IntentMandate:
    """User's initial purchase intent."""
    user_cart_confirmation_required: bool
    natural_language_description: str
    merchants: list[str]
    requires_refundability: bool
    intent_expiry: str
    skus: Optional[list[str]] = None


@dataclass
class PaymentRequest:
    """W3C Payment Request API structure."""
    id: str
    supported_methods: list[str]
    display_items: list[PaymentItem]
    total: PaymentItem


@dataclass
class CartContents:
    """Merchant-signed cart details."""
    id: str
    user_cart_confirmation_required: bool
    payment_request: PaymentRequest
    cart_expiry: str
    merchant_name: str


@dataclass
class CartMandate:
    """Merchant-signed cart."""
    contents: CartContents
    merchant_authorization: Optional[str] = None


@dataclass
class PaymentResponse:
    """User's chosen payment method."""
    method_name: str
    details: Optional[dict] = None
    payer_email: Optional[str] = None
    request_id: Optional[str] = None


@dataclass
class PaymentMandateContents:
    """Payment mandate details."""
    payment_mandate_id: str
    payment_details_id: str
    payment_details_total: PaymentItem
    payment_response: PaymentResponse
    merchant_agent: str
    timestamp: str


@dataclass
class PaymentMandate:
    """User's authorization for payment."""
    payment_mandate_contents: PaymentMandateContents
    user_authorization: Optional[str] = None


@dataclass
class PaymentSuccess:
    """Successful payment details."""
    merchant_confirmation_id: str
    psp_confirmation_id: Optional[str] = None
    network_confirmation_id: Optional[str] = None


@dataclass
class PaymentError:
    """Payment error details."""
    code: str
    message: str


@dataclass
class PaymentReceipt:
    """Final payment result."""
    payment_mandate_id: str
    timestamp: str
    payment_id: str
    amount: Amount
    payment_status: str  # "success", "error", "failure"
    success: Optional[PaymentSuccess] = None
    error: Optional[PaymentError] = None
    payment_method_details: Optional[dict] = None


@dataclass
class BidRequest:
    """Request for payment provider bid."""
    amount: float
    currency: str
    work_category: str
    consumer_id: str
    provider_id: str


@dataclass
class BidResponse:
    """Payment provider bid response."""
    provider_id: str
    provider_name: str
    base_fee_percent: float
    reward_percent: float
    net_fee_percent: float
    processing_time_seconds: int
    supported_methods: list[str]
    fraud_protection: str


@dataclass
class AP2Client:
    """Client for AP2 (Agent Payments Protocol) interactions.

    Supports the full mandate chain:
    1. IntentMandate - Consumer signals purchase intent
    2. CartMandate - Provider signs cart with price
    3. PaymentMandate - Consumer authorizes payment
    4. PaymentReceipt - Confirms transaction complete

    Token Bank Discovery:
    - Discovers Token Bank via AEX Registry (capability: token_banking)
    - Caches the discovered endpoint for subsequent calls
    """

    aex_registry_url: str  # REQUIRED - AEX Registry URL for Token Bank discovery
    _session: Optional[aiohttp.ClientSession] = field(default=None, init=False)
    _discovered_bank_url: Optional[str] = field(default=None, init=False)

    async def _get_session(self) -> aiohttp.ClientSession:
        """Get or create HTTP session."""
        if self._session is None or self._session.closed:
            self._session = aiohttp.ClientSession()
        return self._session

    async def close(self):
        """Close the HTTP session."""
        if self._session and not self._session.closed:
            await self._session.close()

    async def _discover_token_bank(self) -> str:
        """Discover Token Bank via AEX Registry.

        Queries AEX for providers with 'token_banking' capability.

        Returns:
            Token Bank URL from AEX Registry

        Raises:
            RuntimeError: If Token Bank not found in AEX Registry
        """
        # Return cached if available
        if self._discovered_bank_url:
            return self._discovered_bank_url

        # Discover via AEX Registry (REQUIRED)
        try:
            session = await self._get_session()
            logger.info(f"🔍 Discovering Token Bank via AEX Registry: {self.aex_registry_url}")

            async with session.get(
                f"{self.aex_registry_url}/v1/providers",
                params={"capability": "token_banking"},
                timeout=aiohttp.ClientTimeout(total=10)
            ) as resp:
                if resp.status == 200:
                    data = await resp.json()
                    providers = data.get("providers", [])

                    # Filter for providers that have token_banking capability
                    banking_providers = [
                        p for p in providers
                        if "token_banking" in p.get("capabilities", [])
                    ]

                    if banking_providers:
                        # Pick the first token_banking provider
                        bank = banking_providers[0]
                        bank_url = bank.get("endpoint", "")

                        if bank_url:
                            logger.info(f"✅ Discovered Token Bank via AEX: {bank_url}")
                            self._discovered_bank_url = bank_url
                            return bank_url
                        else:
                            raise RuntimeError("Token Bank provider found in AEX but has no endpoint")
                    else:
                        raise RuntimeError("No token_banking providers registered in AEX Registry")
                else:
                    raise RuntimeError(f"AEX Registry query failed with status {resp.status}")

        except aiohttp.ClientError as e:
            raise RuntimeError(f"Failed to connect to AEX Registry: {e}")

    async def get_token_bank_url(self) -> str:
        """Get the Token Bank URL (discovered via AEX)."""
        return await self._discover_token_bank()

    # =========================================
    # Provider Capabilities and Bidding
    # =========================================

    async def get_capabilities(self) -> dict:
        """Get Token Bank AP2 capabilities."""
        session = await self._get_session()
        bank_url = await self._discover_token_bank()
        async with session.get(f"{bank_url}/ap2/capabilities") as resp:
            if resp.status == 200:
                return await resp.json()
            raise Exception(f"Failed to get capabilities: {await resp.text()}")

    async def request_bid(self, bid_request: BidRequest) -> Optional[BidResponse]:
        """Request a bid from the Token Bank payment provider."""
        session = await self._get_session()
        payload = {
            "amount": bid_request.amount,
            "currency": bid_request.currency,
            "work_category": bid_request.work_category,
            "consumer_id": bid_request.consumer_id,
            "provider_id": bid_request.provider_id,
        }

        bank_url = await self._discover_token_bank()
        async with session.post(f"{bank_url}/ap2/bid", json=payload) as resp:
            if resp.status == 200:
                data = await resp.json()
                return BidResponse(
                    provider_id=data["provider_id"],
                    provider_name=data["provider_name"],
                    base_fee_percent=data["base_fee_percent"],
                    reward_percent=data["reward_percent"],
                    net_fee_percent=data["net_fee_percent"],
                    processing_time_seconds=data["processing_time_seconds"],
                    supported_methods=data["supported_methods"],
                    fraud_protection=data["fraud_protection"],
                )
            logger.error(f"Bid request failed: {await resp.text()}")
            return None

    # =========================================
    # Mandate Creation (Step-by-Step)
    # =========================================

    async def create_intent_mandate(
        self,
        consumer_id: str,
        provider_id: str,
        amount: float,
        description: str,
        expires_in: str = "24h",
    ) -> tuple[Optional[IntentMandate], Optional[str]]:
        """Step 1: Create an intent mandate (consumer signals purchase intent)."""
        session = await self._get_session()
        payload = {
            "consumer_id": consumer_id,
            "provider_id": provider_id,
            "amount": amount,
            "description": description,
            "expires_in": expires_in,
        }

        bank_url = await self._discover_token_bank()
        async with session.post(f"{bank_url}/ap2/intent", json=payload) as resp:
            if resp.status == 201:
                data = await resp.json()
                intent_data = data["intent_mandate"]
                mandate = IntentMandate(
                    user_cart_confirmation_required=intent_data["user_cart_confirmation_required"],
                    natural_language_description=intent_data["natural_language_description"],
                    merchants=intent_data.get("merchants", []),
                    requires_refundability=intent_data["requires_refundability"],
                    intent_expiry=intent_data["intent_expiry"],
                )
                return mandate, data["mandate_id"]
            logger.error(f"Intent mandate creation failed: {await resp.text()}")
            return None, None

    async def create_cart_mandate(
        self,
        intent_mandate_id: str,
        items: list[dict],
        total: dict,
        expires_in: str = "15m",
    ) -> tuple[Optional[CartMandate], Optional[str]]:
        """Step 2: Create a cart mandate (provider signs cart with price)."""
        session = await self._get_session()
        payload = {
            "intent_mandate_id": intent_mandate_id,
            "items": items,
            "total": total,
            "expires_in": expires_in,
        }

        bank_url = await self._discover_token_bank()
        async with session.post(f"{bank_url}/ap2/cart", json=payload) as resp:
            if resp.status == 201:
                data = await resp.json()
                cart_data = data["cart_mandate"]
                contents_data = cart_data["contents"]
                pr_data = contents_data["payment_request"]

                payment_request = PaymentRequest(
                    id=pr_data["id"],
                    supported_methods=pr_data["supportedMethods"],
                    display_items=[
                        PaymentItem(
                            label=item["label"],
                            amount=Amount(
                                currency=item["amount"]["currency"],
                                value=item["amount"]["value"],
                            ),
                        )
                        for item in pr_data.get("displayItems", [])
                    ],
                    total=PaymentItem(
                        label=pr_data["total"]["label"],
                        amount=Amount(
                            currency=pr_data["total"]["amount"]["currency"],
                            value=pr_data["total"]["amount"]["value"],
                        ),
                    ),
                )

                contents = CartContents(
                    id=contents_data["id"],
                    user_cart_confirmation_required=contents_data["user_cart_confirmation_required"],
                    payment_request=payment_request,
                    cart_expiry=contents_data["cart_expiry"],
                    merchant_name=contents_data["merchant_name"],
                )

                mandate = CartMandate(
                    contents=contents,
                    merchant_authorization=cart_data.get("merchant_authorization"),
                )
                return mandate, data["mandate_id"]
            logger.error(f"Cart mandate creation failed: {await resp.text()}")
            return None, None

    async def create_payment_mandate(
        self,
        cart_mandate_id: str,
        payment_method: str = "aex-token",
    ) -> tuple[Optional[PaymentMandate], Optional[str]]:
        """Step 3: Create a payment mandate (consumer authorizes payment)."""
        session = await self._get_session()
        payload = {
            "cart_mandate_id": cart_mandate_id,
            "payment_method": payment_method,
        }

        bank_url = await self._discover_token_bank()
        async with session.post(f"{bank_url}/ap2/payment", json=payload) as resp:
            if resp.status == 201:
                data = await resp.json()
                pm_data = data["payment_mandate"]
                contents_data = pm_data["payment_mandate_contents"]

                payment_response = PaymentResponse(
                    method_name=contents_data["payment_response"]["methodName"],
                    details=contents_data["payment_response"].get("details"),
                )

                contents = PaymentMandateContents(
                    payment_mandate_id=contents_data["payment_mandate_id"],
                    payment_details_id=contents_data["payment_details_id"],
                    payment_details_total=PaymentItem(
                        label=contents_data["payment_details_total"]["label"],
                        amount=Amount(
                            currency=contents_data["payment_details_total"]["amount"]["currency"],
                            value=contents_data["payment_details_total"]["amount"]["value"],
                        ),
                    ),
                    payment_response=payment_response,
                    merchant_agent=contents_data["merchant_agent"],
                    timestamp=contents_data["timestamp"],
                )

                mandate = PaymentMandate(
                    payment_mandate_contents=contents,
                    user_authorization=pm_data.get("user_authorization"),
                )
                return mandate, data["mandate_id"]
            logger.error(f"Payment mandate creation failed: {await resp.text()}")
            return None, None

    async def process_payment(
        self,
        payment_mandate: PaymentMandate,
        from_agent_id: str,
        to_agent_id: str,
        amount: float,
        currency: str = "AEX",
        reference: str = "",
        description: str = "",
    ) -> tuple[bool, Optional[PaymentReceipt], Optional[str]]:
        """Step 4: Process the payment and get a receipt."""
        session = await self._get_session()

        # Convert PaymentMandate to dict for JSON serialization
        pm_dict = {
            "payment_mandate_contents": {
                "payment_mandate_id": payment_mandate.payment_mandate_contents.payment_mandate_id,
                "payment_details_id": payment_mandate.payment_mandate_contents.payment_details_id,
                "payment_details_total": {
                    "label": payment_mandate.payment_mandate_contents.payment_details_total.label,
                    "amount": {
                        "currency": payment_mandate.payment_mandate_contents.payment_details_total.amount.currency,
                        "value": payment_mandate.payment_mandate_contents.payment_details_total.amount.value,
                    },
                },
                "payment_response": {
                    "methodName": payment_mandate.payment_mandate_contents.payment_response.method_name,
                    "details": payment_mandate.payment_mandate_contents.payment_response.details or {},
                },
                "merchant_agent": payment_mandate.payment_mandate_contents.merchant_agent,
                "timestamp": payment_mandate.payment_mandate_contents.timestamp,
            },
            "user_authorization": payment_mandate.user_authorization,
        }

        payload = {
            "payment_mandate": pm_dict,
            "from_agent_id": from_agent_id,
            "to_agent_id": to_agent_id,
            "amount": amount,
            "currency": currency,
            "reference": reference,
            "description": description,
        }

        bank_url = await self._discover_token_bank()
        async with session.post(f"{bank_url}/ap2/process", json=payload) as resp:
            data = await resp.json()

            receipt = None
            if "receipt" in data and data["receipt"]:
                r = data["receipt"]
                receipt = PaymentReceipt(
                    payment_mandate_id=r["payment_mandate_id"],
                    timestamp=r["timestamp"],
                    payment_id=r["payment_id"],
                    amount=Amount(
                        currency=r["amount"]["currency"],
                        value=r["amount"]["value"],
                    ),
                    payment_status=r["payment_status"],
                    success=PaymentSuccess(
                        merchant_confirmation_id=r["success"]["merchant_confirmation_id"],
                        psp_confirmation_id=r["success"].get("psp_confirmation_id"),
                        network_confirmation_id=r["success"].get("network_confirmation_id"),
                    ) if r.get("success") else None,
                    error=PaymentError(
                        code=r["error"]["code"],
                        message=r["error"]["message"],
                    ) if r.get("error") else None,
                    payment_method_details=r.get("payment_method_details"),
                )

            return data.get("success", False), receipt, data.get("transaction_id")

    # =========================================
    # Simplified Flow (All-in-One)
    # =========================================

    async def process_mandate_chain(
        self,
        consumer_id: str,
        provider_id: str,
        amount: float,
        description: str,
    ) -> tuple[bool, Optional[PaymentReceipt], Optional[str]]:
        """
        Process the complete AP2 mandate chain in one call.

        This is a simplified flow that handles:
        1. IntentMandate creation
        2. CartMandate creation
        3. PaymentMandate creation
        4. Payment execution
        5. PaymentReceipt generation

        Returns:
            Tuple of (success, receipt, error_message)
        """
        session = await self._get_session()
        payload = {
            "consumer_id": consumer_id,
            "provider_id": provider_id,
            "amount": amount,
            "description": description,
        }

        bank_url = await self._discover_token_bank()
        async with session.post(f"{bank_url}/ap2/process-chain", json=payload) as resp:
            data = await resp.json()

            receipt = None
            if "receipt" in data and data["receipt"]:
                r = data["receipt"]
                receipt = PaymentReceipt(
                    payment_mandate_id=r["payment_mandate_id"],
                    timestamp=r["timestamp"],
                    payment_id=r["payment_id"],
                    amount=Amount(
                        currency=r["amount"]["currency"],
                        value=r["amount"]["value"],
                    ),
                    payment_status=r["payment_status"],
                    success=PaymentSuccess(
                        merchant_confirmation_id=r["success"]["merchant_confirmation_id"],
                        psp_confirmation_id=r["success"].get("psp_confirmation_id"),
                        network_confirmation_id=r["success"].get("network_confirmation_id"),
                    ) if r.get("success") else None,
                    error=PaymentError(
                        code=r["error"]["code"],
                        message=r["error"]["message"],
                    ) if r.get("error") else None,
                    payment_method_details=r.get("payment_method_details"),
                )

            success = data.get("success", False)
            error = data.get("error") if not success else None

            return success, receipt, error

    # =========================================
    # Mandate Queries
    # =========================================

    async def list_mandates(
        self,
        agent_id: str,
        mandate_type: Optional[str] = None,
    ) -> list[dict]:
        """List all mandates for an agent."""
        session = await self._get_session()
        params = {}
        if mandate_type:
            params["type"] = mandate_type

        bank_url = await self._discover_token_bank()
        async with session.get(
            f"{bank_url}/ap2/mandates/{agent_id}",
            params=params,
        ) as resp:
            if resp.status == 200:
                data = await resp.json()
                return data.get("mandates", [])
            return []

    async def get_mandate(self, agent_id: str, mandate_id: str) -> Optional[dict]:
        """Get a specific mandate by ID."""
        session = await self._get_session()
        bank_url = await self._discover_token_bank()
        async with session.get(
            f"{bank_url}/ap2/mandates/{agent_id}/{mandate_id}"
        ) as resp:
            if resp.status == 200:
                return await resp.json()
            return None

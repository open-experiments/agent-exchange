"""Base payment provider agent implementation with AP2 support."""

import hashlib
import json
import logging
import time
import uuid
from abc import abstractmethod
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone
from typing import Any, AsyncIterator, Optional

from .a2a_server import A2AHandler, Message, TaskState
from .aex_client import AEXClient
from .config import AgentConfig

logger = logging.getLogger(__name__)


# AP2 Mandate Keys (from Google's AP2 spec)
CART_MANDATE_DATA_KEY = "ap2.mandates.CartMandate"
INTENT_MANDATE_DATA_KEY = "ap2.mandates.IntentMandate"
PAYMENT_MANDATE_DATA_KEY = "ap2.mandates.PaymentMandate"


@dataclass
class PaymentBid:
    """A bid from a payment provider."""
    provider_id: str
    provider_name: str
    base_fee_percent: float
    reward_percent: float
    net_fee_percent: float  # base_fee - reward
    processing_time_seconds: int
    supported_methods: list[str]
    fraud_protection: str  # "none", "basic", "advanced"


@dataclass
class PaymentRequest:
    """A payment request to process."""
    payment_id: str
    amount: float
    currency: str
    work_category: str  # "contracts", "compliance", "general"
    consumer_id: str
    provider_id: str
    contract_id: str
    payment_method: str  # "card", "bank", "crypto", "aex_balance"


@dataclass
class PaymentResult:
    """Result of payment processing."""
    success: bool
    payment_id: str
    transaction_id: str
    receipt_id: str
    amount_charged: float
    fee_amount: float
    reward_amount: float
    net_cost: float
    processing_time_ms: int
    error_message: Optional[str] = None


# AP2 Mandate Classes
@dataclass
class AP2PaymentItem:
    """W3C PaymentItem for AP2."""
    label: str
    amount: dict  # {"currency": "USD", "value": 25.00}
    refund_period: int = 30  # days


@dataclass
class AP2PaymentRequest:
    """W3C PaymentRequest for AP2."""
    id: str
    display_items: list[AP2PaymentItem]
    total: AP2PaymentItem
    method_data: list[dict]  # Supported payment methods


@dataclass
class AP2CartContents:
    """AP2 Cart Contents signed by payment provider."""
    id: str
    user_cart_confirmation_required: bool
    payment_request: AP2PaymentRequest
    cart_expiry: str  # ISO 8601
    merchant_name: str


@dataclass
class AP2CartMandate:
    """AP2 Cart Mandate - signed cart contents."""
    contents: AP2CartContents
    merchant_authorization: str  # JWT signature


@dataclass
class AP2PaymentReceipt:
    """AP2 Payment Receipt."""
    receipt_id: str
    transaction_id: str
    payment_mandate_id: str
    amount: dict  # {"currency": "USD", "value": 25.00}
    status: str  # "SUCCESS", "FAILED", "PENDING"
    timestamp: str  # ISO 8601
    provider_id: str
    provider_name: str
    error_message: Optional[str] = None


@dataclass
class BasePaymentAgent(A2AHandler):
    """Base class for payment provider agents."""

    config: AgentConfig
    aex_client: Optional[AEXClient] = None

    # Payment provider characteristics - override in subclasses
    base_fee_percent: float = 2.0
    processing_time_seconds: int = 5
    supported_methods: list[str] = field(default_factory=lambda: ["card"])
    fraud_protection: str = "basic"

    # Category-specific rewards - override in subclasses
    # Maps work category to reward percentage
    category_rewards: dict[str, float] = field(default_factory=dict)

    def __post_init__(self):
        """Initialize the payment agent."""
        import os
        if self.config.aex.enabled:
            self.aex_client = AEXClient(
                gateway_url=self.config.aex.gateway_url,
                api_key=os.environ.get("AEX_API_KEY", "dev-api-key"),
            )
        # Provider ID derived from name
        self.provider_id = self.config.name.lower().replace(" ", "")

    def generate_cart_mandate(self, amount: float, currency: str, description: str,
                              work_category: str, contract_id: str) -> AP2CartMandate:
        """Generate an AP2 Cart Mandate for a payment."""
        cart_id = f"cart_{contract_id}_{uuid.uuid4().hex[:8]}"
        payment_request_id = f"pay_req_{contract_id}"

        # Calculate fees
        reward = self.get_reward_for_category(work_category)
        fee_amount = amount * (self.base_fee_percent / 100)
        reward_amount = amount * (reward / 100)
        net_fee = fee_amount - reward_amount
        total_with_fee = amount + net_fee

        # Build W3C PaymentRequest
        payment_request = AP2PaymentRequest(
            id=payment_request_id,
            display_items=[
                AP2PaymentItem(
                    label=description or "Legal Service",
                    amount={"currency": currency, "value": amount},
                ),
                AP2PaymentItem(
                    label=f"Processing Fee ({self.base_fee_percent}%)",
                    amount={"currency": currency, "value": round(fee_amount, 2)},
                ),
                AP2PaymentItem(
                    label=f"{work_category.title()} Reward (-{reward}%)",
                    amount={"currency": currency, "value": round(-reward_amount, 2)},
                ),
            ],
            total=AP2PaymentItem(
                label="Total",
                amount={"currency": currency, "value": round(total_with_fee, 2)},
            ),
            method_data=[
                {"supported_methods": method} for method in self.supported_methods
            ],
        )

        # Build cart contents
        cart_expiry = (datetime.now(timezone.utc) + timedelta(minutes=15)).isoformat()
        contents = AP2CartContents(
            id=cart_id,
            user_cart_confirmation_required=True,
            payment_request=payment_request,
            cart_expiry=cart_expiry,
            merchant_name=self.config.name,
        )

        # Generate merchant authorization (simplified JWT-like signature)
        cart_hash = self._hash_contents(contents)
        merchant_auth = self._generate_merchant_authorization(cart_hash)

        return AP2CartMandate(
            contents=contents,
            merchant_authorization=merchant_auth,
        )

    def _hash_contents(self, contents: AP2CartContents) -> str:
        """Create SHA256 hash of cart contents."""
        # Serialize to JSON for hashing
        data = json.dumps({
            "id": contents.id,
            "user_cart_confirmation_required": contents.user_cart_confirmation_required,
            "payment_request_id": contents.payment_request.id,
            "cart_expiry": contents.cart_expiry,
            "merchant_name": contents.merchant_name,
        }, sort_keys=True)
        return hashlib.sha256(data.encode()).hexdigest()

    def _generate_merchant_authorization(self, cart_hash: str) -> str:
        """Generate a simplified merchant authorization token."""
        # In production, this would be a proper JWT with RSA/ECDSA signing
        auth_data = {
            "iss": self.provider_id,
            "iat": int(time.time()),
            "exp": int(time.time()) + 900,  # 15 minutes
            "cart_hash": cart_hash,
        }
        return json.dumps(auth_data)

    def generate_payment_receipt(self, result: PaymentResult, currency: str) -> AP2PaymentReceipt:
        """Generate an AP2 Payment Receipt."""
        return AP2PaymentReceipt(
            receipt_id=result.receipt_id,
            transaction_id=result.transaction_id,
            payment_mandate_id=f"pm_{result.payment_id}",
            amount={"currency": currency, "value": result.amount_charged},
            status="SUCCESS" if result.success else "FAILED",
            timestamp=datetime.now(timezone.utc).isoformat(),
            provider_id=self.provider_id,
            provider_name=self.config.name,
            error_message=result.error_message,
        )

    def get_reward_for_category(self, category: str) -> float:
        """Get reward percentage for a work category."""
        return self.category_rewards.get(category, self.category_rewards.get("default", 1.0))

    def calculate_payment_bid(self, amount: float, work_category: str) -> PaymentBid:
        """Calculate a payment bid for the given amount and category."""
        reward = self.get_reward_for_category(work_category)
        net_fee = self.base_fee_percent - reward

        return PaymentBid(
            provider_id=self.config.name.lower().replace(" ", "-"),
            provider_name=self.config.name,
            base_fee_percent=self.base_fee_percent,
            reward_percent=reward,
            net_fee_percent=net_fee,
            processing_time_seconds=self.processing_time_seconds,
            supported_methods=self.supported_methods,
            fraud_protection=self.fraud_protection,
        )

    async def process_payment(self, request: PaymentRequest) -> PaymentResult:
        """Process a payment request."""
        import asyncio
        import time
        import uuid

        start_time = time.time()

        # Simulate processing delay
        await asyncio.sleep(self.processing_time_seconds * 0.1)  # Scaled down for demo

        # Calculate fees and rewards
        reward = self.get_reward_for_category(request.work_category)
        fee_amount = request.amount * (self.base_fee_percent / 100)
        reward_amount = request.amount * (reward / 100)
        net_cost = fee_amount - reward_amount

        processing_time_ms = int((time.time() - start_time) * 1000)

        return PaymentResult(
            success=True,
            payment_id=request.payment_id,
            transaction_id=f"txn_{uuid.uuid4().hex[:12]}",
            receipt_id=f"rcpt_{uuid.uuid4().hex[:12]}",
            amount_charged=request.amount + net_cost,
            fee_amount=fee_amount,
            reward_amount=reward_amount,
            net_cost=net_cost,
            processing_time_ms=processing_time_ms,
        )

    async def handle_message(
        self,
        task_id: str,
        session_id: str,
        message: Message,
        context: dict,
    ) -> AsyncIterator[dict]:
        """Handle incoming A2A message for payment processing."""
        yield {"type": "status", "state": TaskState.WORKING.value, "message": "Processing payment request..."}

        # Extract request from message
        text_content = ""
        for part in message.parts:
            if part.get("type") == "text":
                text_content += part.get("text", "")

        try:
            import json
            request_data = json.loads(text_content)

            # Determine action
            action = request_data.get("action", "process")

            if action == "bid":
                # Return a payment bid with AP2 cart mandate
                amount = request_data.get("amount", 0)
                category = request_data.get("work_category", "general")
                currency = request_data.get("currency", "USD")
                contract_id = request_data.get("contract_id", task_id)
                description = request_data.get("description", "Legal Service")

                bid = self.calculate_payment_bid(amount, category)

                # Generate AP2 Cart Mandate
                cart_mandate = self.generate_cart_mandate(
                    amount=amount,
                    currency=currency,
                    description=description,
                    work_category=category,
                    contract_id=contract_id,
                )

                yield {
                    "type": "result",
                    "parts": [{"type": "text", "text": json.dumps({
                        "action": "bid_response",
                        "bid": {
                            "provider_id": bid.provider_id,
                            "provider_name": bid.provider_name,
                            "base_fee_percent": bid.base_fee_percent,
                            "reward_percent": bid.reward_percent,
                            "net_fee_percent": bid.net_fee_percent,
                            "processing_time_seconds": bid.processing_time_seconds,
                            "supported_methods": bid.supported_methods,
                            "fraud_protection": bid.fraud_protection,
                        },
                        # AP2 Cart Mandate
                        CART_MANDATE_DATA_KEY: {
                            "contents": {
                                "id": cart_mandate.contents.id,
                                "user_cart_confirmation_required": cart_mandate.contents.user_cart_confirmation_required,
                                "payment_request": {
                                    "id": cart_mandate.contents.payment_request.id,
                                    "display_items": [
                                        {"label": item.label, "amount": item.amount}
                                        for item in cart_mandate.contents.payment_request.display_items
                                    ],
                                    "total": {
                                        "label": cart_mandate.contents.payment_request.total.label,
                                        "amount": cart_mandate.contents.payment_request.total.amount,
                                    },
                                    "method_data": cart_mandate.contents.payment_request.method_data,
                                },
                                "cart_expiry": cart_mandate.contents.cart_expiry,
                                "merchant_name": cart_mandate.contents.merchant_name,
                            },
                            "merchant_authorization": cart_mandate.merchant_authorization,
                        }
                    })}],
                }

            elif action == "process":
                # Process the payment with AP2 receipt
                currency = request_data.get("currency", "USD")
                request = PaymentRequest(
                    payment_id=request_data.get("payment_id", f"pay_{task_id}"),
                    amount=request_data.get("amount", 0),
                    currency=currency,
                    work_category=request_data.get("work_category", "general"),
                    consumer_id=request_data.get("consumer_id", ""),
                    provider_id=request_data.get("provider_id", ""),
                    contract_id=request_data.get("contract_id", ""),
                    payment_method=request_data.get("payment_method", "card"),
                )

                result = await self.process_payment(request)

                # Generate AP2 Payment Receipt
                receipt = self.generate_payment_receipt(result, currency)

                yield {
                    "type": "result",
                    "parts": [{"type": "text", "text": json.dumps({
                        "action": "payment_result",
                        "result": {
                            "success": result.success,
                            "payment_id": result.payment_id,
                            "transaction_id": result.transaction_id,
                            "receipt_id": result.receipt_id,
                            "amount_charged": result.amount_charged,
                            "fee_amount": result.fee_amount,
                            "reward_amount": result.reward_amount,
                            "net_cost": result.net_cost,
                            "processing_time_ms": result.processing_time_ms,
                            "provider_name": self.config.name,
                        },
                        # AP2 Payment Receipt
                        "ap2.payment.receipt": {
                            "receipt_id": receipt.receipt_id,
                            "transaction_id": receipt.transaction_id,
                            "payment_mandate_id": receipt.payment_mandate_id,
                            "amount": receipt.amount,
                            "status": receipt.status,
                            "timestamp": receipt.timestamp,
                            "provider_id": receipt.provider_id,
                            "provider_name": receipt.provider_name,
                        }
                    })}],
                }

            else:
                yield {
                    "type": "result",
                    "parts": [{"type": "text", "text": json.dumps({
                        "error": f"Unknown action: {action}",
                        "supported_actions": ["bid", "process"]
                    })}],
                }

        except json.JSONDecodeError:
            yield {
                "type": "result",
                "parts": [{"type": "text", "text": json.dumps({
                    "error": "Invalid JSON in request",
                    "expected_format": {
                        "action": "bid|process",
                        "amount": 25.00,
                        "work_category": "contracts|compliance|general",
                    }
                })}],
            }
        except Exception as e:
            logger.exception(f"Error processing payment: {e}")
            yield {
                "type": "result",
                "parts": [{"type": "text", "text": json.dumps({
                    "error": str(e)
                })}],
            }

    async def register_with_aex(self, base_url: str):
        """Register this payment provider with AEX.

        Payment providers register with:
        - AP2 extension enabled for payment mandate support
        - Payment-specific capabilities
        - Category subscription for payment work
        """
        if not self.aex_client or not self.config.aex.auto_register:
            return

        try:
            # Build capabilities including AP2 support
            capabilities = [
                "payment",
                "payment_processing",
                "ap2_mandates",  # Supports AP2 Cart/Payment mandates
            ]

            # Add category-specific capabilities
            for category in self.category_rewards.keys():
                if category != "default":
                    capabilities.append(f"payment_{category}")

            # Register as a payment provider with AP2 extension
            await self.aex_client.register_provider(
                name=self.config.name,
                description=self.config.description,
                endpoint=base_url,
                bid_webhook=f"{base_url}/a2a",  # Use A2A endpoint for bids
                capabilities=capabilities,
                metadata={
                    "provider_type": "payment",
                    "base_fee_percent": self.base_fee_percent,
                    "fraud_protection": self.fraud_protection,
                    "supported_methods": self.supported_methods,
                    "category_rewards": self.category_rewards,
                    "ap2_enabled": True,
                    "trust_tier": self.config.aex.trust_tier,
                    "trust_score": self.config.aex.trust_score,
                    "extensions": [{
                        "uri": "tag:google.com,2024:ap2",
                        "required": True,
                        "description": "Agent Payments Protocol support"
                    }]
                }
            )

            # Subscribe to payment category
            await self.aex_client.subscribe_to_categories(
                categories=["payment", "payment/*", "settlement"],
                webhook_url=f"{base_url}/a2a",
            )

            logger.info(f"Registered payment provider with AEX: {self.config.name}")
            logger.info(f"  - AP2 enabled: True")
            logger.info(f"  - Base fee: {self.base_fee_percent}%")
            logger.info(f"  - Categories: {list(self.category_rewards.keys())}")

        except Exception as e:
            logger.warning(f"Failed to register with AEX: {e}")

    async def close(self):
        """Cleanup resources."""
        if self.aex_client:
            await self.aex_client.close()

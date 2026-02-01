"""Moltbot Customer Agent - Requests services from other agents via Moltbook.com.

Flow:
1. Search for providers via Moltbook.com (social platform for AI agents)
2. Negotiate service terms via Moltbook (posts/comments)
3. Discover Token Bank via AEX Registry
4. Pay via AP2 to Token Bank
5. Share payment confirmation via Moltbook
6. Receive service delivery
"""

import json
import logging
import os
from typing import AsyncIterator, Optional

import aiohttp

import sys
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..'))

from common.moltbot_agent import MoltbotAgent, AgentConfig

logger = logging.getLogger(__name__)


class CustomerAgent(MoltbotAgent):
    """Customer agent that requests services from other agents via Moltbook.com.

    Complete Flow:
    1. 🔍 MOLTBOOK: Search for service providers by capability
    2. 💬 MOLTBOOK: Negotiate terms (via posts/comments)
    3. 🏦 AEX: Discover Token Bank for payments
    4. 💰 AP2: Execute payment to Token Bank
    5. 📢 MOLTBOOK: Share payment confirmation with provider
    6. 📦 A2A: Receive service delivery from provider
    """

    def get_capabilities(self) -> list[str]:
        return ["consumer", "service_requester"]

    async def _discover_provider_via_moltbook(self, capability: str) -> Optional[dict]:
        """Discover a service provider via Moltbook.com.

        Args:
            capability: Service capability to find (e.g., "research", "writing")

        Returns:
            Provider info with endpoint

        Raises:
            RuntimeError: If provider not found on Moltbook
        """
        if not self.moltbook_client:
            raise RuntimeError("Moltbook client not initialized")

        logger.info(f"🔍 Searching Moltbook for '{capability}' providers...")

        # Search for providers on Moltbook
        providers = await self.moltbook_client.discover_providers(capability)

        if providers:
            provider = providers[0]  # Pick first match
            logger.info(
                f"✅ Found provider via Moltbook: {provider.get('agent_name')} "
                f"at {provider.get('endpoint')}"
            )
            return provider

        raise RuntimeError(f"No providers found on Moltbook for capability: {capability}")

    async def _call_provider_a2a(
        self,
        endpoint: str,
        request_data: dict
    ) -> Optional[dict]:
        """Make a direct A2A HTTP call to a provider.

        Args:
            endpoint: Provider's HTTP endpoint
            request_data: Request payload

        Returns:
            Response from provider
        """
        try:
            session = await self._get_http_session()
            async with session.post(
                f"{endpoint}/message",
                json=request_data,
                headers={"Content-Type": "application/json"},
                timeout=aiohttp.ClientTimeout(total=120)
            ) as resp:
                if resp.status == 200:
                    # Handle SSE response
                    result_data = None
                    async for line in resp.content:
                        line = line.decode('utf-8').strip()
                        if line.startswith('data: '):
                            data = line[6:]
                            if data == '[DONE]':
                                break
                            try:
                                parsed = json.loads(data)
                                if parsed.get('type') == 'result':
                                    # Extract the actual result
                                    parts = parsed.get('parts', [])
                                    if parts:
                                        text = parts[0].get('text', '{}')
                                        result_data = json.loads(text)
                            except json.JSONDecodeError:
                                continue
                    return result_data
                else:
                    error = await resp.text()
                    logger.error(f"A2A call failed ({resp.status}): {error}")
                    return {"error": error}

        except Exception as e:
            logger.error(f"A2A call error: {e}")
            return {"error": str(e)}

    async def request_service_via_moltbook(
        self,
        service_type: str,
        request_details: dict,
    ) -> Optional[dict]:
        """
        Complete workflow for requesting a service via Moltbook.com:

        1. 🔍 DISCOVER: Search Moltbook for providers with capability
        2. 📋 GET ENDPOINT: Get provider's A2A endpoint from Moltbook profile
        3. 💬 NEGOTIATE: Call provider's endpoint to get price
        4. 💰 PAY: Execute AP2 payment to Token Bank
        5. 📦 DELIVER: Call provider with payment proof, get service result
        """
        logger.info(f"🚀 Starting service request: {service_type}")

        # Step 1: Discover provider via Moltbook
        provider = await self._discover_provider_via_moltbook(service_type)
        if not provider:
            return {"error": f"No provider found for capability: {service_type}"}

        target_agent_id = provider.get("agent_id")
        endpoint = provider.get("endpoint")
        expected_price = provider.get("price", 0)

        logger.info(f"📍 Provider: {target_agent_id} at {endpoint}")

        # Step 2: Get price from provider (optional - can use expected_price)
        price = expected_price
        logger.info(f"💵 Service price: {price} AEX")

        # Step 3: Check if we can afford it
        balance = await self.get_balance()
        if balance < price:
            return {
                "error": f"Insufficient funds. Have {balance} AEX, need {price} AEX",
                "balance": balance,
                "required": price,
            }
        logger.info(f"✅ Can afford: {balance} AEX >= {price} AEX")

        # Step 4: Pay via AP2
        logger.info(f"💳 Initiating AP2 payment: {price} AEX to {target_agent_id}")

        success, receipt, error = await self.ap2_client.process_mandate_chain(
            consumer_id=self.config.agent_id,
            provider_id=target_agent_id,
            amount=price,
            description=f"Payment for {service_type} service",
        )

        if not success:
            logger.error(f"❌ Payment failed: {error}")
            return {"error": f"Payment failed: {error}"}

        payment_id = receipt.payment_id if receipt else "unknown"
        logger.info(f"✅ Payment successful: {payment_id}")

        # Step 5: Request service with payment proof
        logger.info(f"📦 Requesting service from {target_agent_id}...")

        service_request = {
            "from_agent": self.config.agent_id,
            "payment_id": payment_id,
            "payment_amount": price,
            **request_details,
        }

        result = await self._call_provider_a2a(endpoint, service_request)

        if result and "error" not in result:
            logger.info("✅ Service delivered successfully!")

            # Post about successful transaction on Moltbook (optional)
            if self.moltbook_client and self.moltbook_client.is_registered:
                await self.moltbook_client.create_post(
                    title=f"Completed {service_type} service with {target_agent_id}",
                    content=f"Just used {target_agent_id} for {service_type}. Paid {price} AEX. Great service!",
                    submolt="transactions"
                )

            return {
                "success": True,
                "provider": target_agent_id,
                "service_type": service_type,
                "payment": {
                    "amount": price,
                    "payment_id": payment_id,
                },
                "result": result,
            }
        else:
            logger.error(f"❌ Service request failed: {result}")
            return {
                "error": "Service delivery failed",
                "payment_made": True,
                "payment_id": payment_id,
                "provider_response": result,
            }

    async def handle_request(self, request: dict) -> AsyncIterator[dict]:
        """Handle incoming requests.

        Customer agent can:
        - Request services from other agents via Moltbook discovery
        - Check balances
        - But does NOT provide services itself

        Service Request Flow (via Moltbook.com):
        1. 🔍 Discover provider on Moltbook by capability
        2. 📋 Get provider's A2A endpoint from profile
        3. 💰 Pay via AP2 to Token Bank
        4. 📦 Call provider's endpoint with payment proof
        5. ✅ Receive service result
        """
        action = request.get("action", "")

        if action == "request_service":
            # Customer requests a service from another agent
            service_type = request.get("service_type", "research")
            request_details = request.get("request_details", {})

            yield {
                "type": "status",
                "state": "working",
                "message": f"🔍 Discovering {service_type} provider via Moltbook..."
            }

            result = await self.request_service_via_moltbook(
                service_type=service_type,
                request_details=request_details,
            )

            yield {
                "type": "result",
                "parts": [{
                    "type": "text",
                    "text": json.dumps(result, indent=2) if result else json.dumps({"error": "No result"})
                }]
            }

        elif action == "discover":
            # Just discover providers without requesting service
            capability = request.get("capability", "research")

            yield {
                "type": "status",
                "state": "working",
                "message": f"🔍 Searching Moltbook for '{capability}' providers..."
            }

            provider = await self._discover_provider_via_moltbook(capability)

            yield {
                "type": "result",
                "parts": [{
                    "type": "text",
                    "text": json.dumps({
                        "capability": capability,
                        "provider": provider,
                        "discovery_method": "moltbook" if self.moltbook_client else "fallback",
                    }, indent=2)
                }]
            }

        elif action == "balance":
            # Check balance
            balance = await self.get_balance()
            yield {
                "type": "result",
                "parts": [{
                    "type": "text",
                    "text": json.dumps({
                        "agent_id": self.config.agent_id,
                        "balance": balance,
                        "token_type": "AEX",
                    }, indent=2)
                }]
            }

        else:
            # Customer agent does not provide services
            yield {
                "type": "result",
                "parts": [{
                    "type": "text",
                    "text": json.dumps({
                        "error": "Customer agent does not provide services",
                        "hint": "Use action='request_service' to request services from other agents",
                        "available_actions": ["request_service", "discover", "balance"],
                        "example": {
                            "action": "request_service",
                            "service_type": "research",
                            "request_details": {"query": "AI trends 2025"}
                        },
                        "capabilities": self.get_capabilities(),
                        "balance": await self.get_balance(),
                    }, indent=2)
                }]
            }


def create_agent() -> CustomerAgent:
    """Create and configure the customer agent."""
    config = AgentConfig(
        agent_id="moltbot-customer",
        agent_name="Moltbot Customer",
        description="AI customer agent that requests services from other agents via Molt.bot",
        port=int(os.environ.get("PORT", "8099")),
        initial_tokens=float(os.environ.get("INITIAL_TOKENS", "200")),
        service_price=float(os.environ.get("SERVICE_PRICE", "0")),  # Customer doesn't sell
        token_bank_url=os.environ.get("TOKEN_BANK_URL", "http://aex-token-bank:8094"),
        aex_registry_url=os.environ.get("AEX_REGISTRY_URL", "http://aex-provider-registry:8080"),
        openclaw_gateway_url=os.environ.get("OPENCLAW_GATEWAY_URL", "ws://localhost:18789"),
        enable_gateway=os.environ.get("ENABLE_GATEWAY", "true").lower() == "true",
        enable_ap2=os.environ.get("ENABLE_AP2", "true").lower() == "true",
        bank_token=os.environ.get("BANK_TOKEN", ""),
        # Moltbook integration
        enable_moltbook=os.environ.get("ENABLE_MOLTBOOK", "true").lower() == "true",
        moltbook_api_key=os.environ.get("MOLTBOOK_API_KEY", ""),
        moltbook_base_url=os.environ.get("MOLTBOOK_BASE_URL", ""),
        # LLM integration (customer doesn't provide services, so no LLM needed)
        enable_llm=False,
        agent_type="general",
    )
    return CustomerAgent(config=config)

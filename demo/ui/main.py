"""AEX Demo UI - Dashboard Style with Visual Topology."""

import json
import os
import time
import random
import httpx
import mesop as me
from dataclasses import field
from datetime import datetime

# Configuration
AEX_GATEWAY_URL = os.environ.get("AEX_GATEWAY_URL", "http://localhost:8080")
AEX_SETTLEMENT_URL = os.environ.get("AEX_SETTLEMENT_URL", "http://localhost:8088")
AEX_PROVIDER_REGISTRY_URL = os.environ.get("AEX_PROVIDER_REGISTRY_URL", "http://localhost:8085")
AEX_WORK_PUBLISHER_URL = os.environ.get("AEX_WORK_PUBLISHER_URL", "http://localhost:8081")
AEX_BID_GATEWAY_URL = os.environ.get("AEX_BID_GATEWAY_URL", "http://localhost:8082")
AEX_CONTRACT_ENGINE_URL = os.environ.get("AEX_CONTRACT_ENGINE_URL", "http://localhost:8084")
LEGAL_AGENT_A_URL = os.environ.get("LEGAL_AGENT_A_URL", "http://localhost:8100")
LEGAL_AGENT_B_URL = os.environ.get("LEGAL_AGENT_B_URL", "http://localhost:8101")
LEGAL_AGENT_C_URL = os.environ.get("LEGAL_AGENT_C_URL", "http://localhost:8102")

# Payment Provider URLs
LEGALPAY_URL = os.environ.get("LEGALPAY_URL", "http://localhost:8200")
CONTRACTPAY_URL = os.environ.get("CONTRACTPAY_URL", "http://localhost:8201")
COMPLIANCEPAY_URL = os.environ.get("COMPLIANCEPAY_URL", "http://localhost:8202")

# Provider ID to URL mapping for execution
PROVIDER_URL_MAP = {
    "budget-legal-ai": LEGAL_AGENT_A_URL,
    "standard-legal-ai": LEGAL_AGENT_B_URL,
    "premium-legal-ai": LEGAL_AGENT_C_URL,
    "legal-agent-a": LEGAL_AGENT_A_URL,
    "legal-agent-b": LEGAL_AGENT_B_URL,
    "legal-agent-c": LEGAL_AGENT_C_URL,
}


@me.stateclass
class Bid:
    """A bid from a provider."""
    provider_id: str = ""
    provider_name: str = ""
    price: float = 0.0
    confidence: float = 0.0
    estimated_minutes: int = 0
    trust_score: float = 0.0
    tier: str = ""
    score: float = 0.0
    a2a_endpoint: str = ""  # A2A endpoint for this provider


@me.stateclass
class TaskResult:
    """Result of a completed task."""
    task_id: str = ""
    description: str = ""
    document_pages: int = 5
    bid_strategy: str = "balanced"
    bids: list[dict] = field(default_factory=list)  # All bids received
    winner_name: str = ""
    winner_tier: str = ""
    winner_price: float = 0.0
    winner_score: float = 0.0
    contract_id: str = ""
    agent_response: str = ""
    execution_time_ms: int = 0
    platform_fee: float = 0.0
    provider_payout: float = 0.0
    timestamp: str = ""
    status: str = "pending"  # pending, bidding, evaluating, awarded, executing, paying, completed, failed
    current_step: int = 0  # 1-7 for progress tracking
    # AP2 Payment fields
    ap2_cart_mandate_id: str = ""
    ap2_payment_provider: str = ""
    ap2_payment_method: str = ""
    ap2_payment_receipt_id: str = ""


@me.stateclass
class State:
    """Application state."""
    step: int = 1
    work_description: str = ""
    document_pages: int = 5
    bid_strategy: str = "balanced"
    work_id: str = ""
    bids: list[Bid] = field(default_factory=list)
    bidding_complete: bool = False
    selected_bid_index: int = 0  # Manual override for winner selection
    winner_provider_id: str = ""
    winner_provider_name: str = ""
    winner_tier: str = ""
    winner_a2a_endpoint: str = ""
    contract_id: str = ""
    agent_response: str = ""
    execution_time_ms: int = 0
    agreed_price: float = 0.0
    platform_fee: float = 0.0
    provider_payout: float = 0.0
    settlement_timestamp: str = ""
    loading: bool = False
    error: str = ""
    # AP2 Payment fields
    ap2_enabled: bool = False
    payment_mandate_id: str = ""
    payment_receipt_id: str = ""
    payment_transaction_id: str = ""
    payment_method: str = ""
    # Payment Provider fields (for payment provider marketplace)
    payment_provider_bids: list[dict] = field(default_factory=list)
    selected_payment_provider: dict = field(default_factory=dict)
    payment_base_fee: float = 0.0
    payment_reward: float = 0.0
    payment_net_cost: float = 0.0
    work_category: str = ""
    # Stats (start at 0, fetched from AEX or incremented during demo runs)
    total_transactions: int = 0
    successful_transactions: int = 0
    platform_revenue: float = 0.0
    avg_response_time: int = 0
    total_response_time: int = 0
    # Registered providers from AEX
    registered_providers: list[dict] = field(default_factory=list)
    # Provider group expansion state
    legal_providers_expanded: bool = True
    payment_providers_expanded: bool = True
    # Output log for simplified UI
    log_messages: list[str] = field(default_factory=list)
    is_running: bool = False
    # Task history - stores all completed tasks with their results
    task_results: list[TaskResult] = field(default_factory=list)
    # Current task being processed (for real-time updates)
    current_task: TaskResult = field(default_factory=TaskResult)


def fetch_registered_providers() -> list[dict]:
    """Fetch registered providers from AEX Provider Registry."""
    try:
        with httpx.Client(timeout=10.0) as client:
            resp = client.get(f"{AEX_PROVIDER_REGISTRY_URL}/v1/providers")
            if resp.status_code == 200:
                data = resp.json()
                providers = data.get("providers", [])
                # Normalize provider data and deduplicate by name
                # Keep the provider with more capabilities (more recent registration)
                by_name = {}
                for p in providers:
                    name = p.get("name", "Unknown Agent")
                    capabilities = p.get("capabilities", [])
                    # If we already have this provider, keep the one with more capabilities
                    if name in by_name:
                        if len(capabilities) > len(by_name[name].get("capabilities", [])):
                            by_name[name] = p
                    else:
                        by_name[name] = p

                # Normalize the deduplicated providers
                result = []
                for p in by_name.values():
                    capabilities = p.get("capabilities", [])
                    # Determine provider type based on capabilities
                    is_payment = "payment" in capabilities or "payment_processing" in capabilities
                    result.append({
                        "provider_id": p.get("provider_id", ""),
                        "name": p.get("name", "Unknown Agent"),
                        "description": p.get("description", ""),
                        "endpoint": p.get("endpoint", ""),
                        "trust_score": p.get("trust_score", 0.5),
                        "trust_tier": p.get("trust_tier", "UNVERIFIED"),
                        "status": p.get("status", "ACTIVE"),
                        "capabilities": capabilities,
                        "provider_type": "payment" if is_payment else "legal",
                    })
                return result
    except Exception as e:
        print(f"Error fetching providers: {e}")
    return []


def check_provider_health(endpoint: str) -> bool:
    """Check if a provider is healthy/online."""
    try:
        with httpx.Client(timeout=3.0) as client:
            resp = client.get(f"{endpoint}/health")
            return resp.status_code == 200
    except:
        return False


def on_page_load(e: me.LoadEvent):
    """Called when the page loads - fetch providers from registry."""
    state = me.state(State)
    state.registered_providers = fetch_registered_providers()


def on_refresh_providers(e: me.ClickEvent):
    """Refresh the list of registered providers."""
    state = me.state(State)
    state.registered_providers = fetch_registered_providers()


def on_toggle_legal_providers(e: me.ClickEvent):
    """Toggle legal providers group expansion."""
    state = me.state(State)
    state.legal_providers_expanded = not state.legal_providers_expanded


def on_toggle_payment_providers(e: me.ClickEvent):
    """Toggle payment providers group expansion."""
    state = me.state(State)
    state.payment_providers_expanded = not state.payment_providers_expanded


def fetch_settlement_info(contract_id: str) -> dict:
    """Fetch settlement info from AEX Settlement service."""
    try:
        with httpx.Client(timeout=10.0) as client:
            resp = client.get(f"{AEX_SETTLEMENT_URL}/v1/contracts/{contract_id}/settlement")
            if resp.status_code == 200:
                return resp.json()
    except Exception as e:
        print(f"Error fetching settlement: {e}")
    return {}


def publish_work_to_aex(description: str, document_pages: int, max_budget: float) -> dict:
    """Publish work request to AEX Work Publisher service."""
    try:
        payload = {
            "consumer_id": "consumer-demo-001",
            "category": "legal/contract_review",
            "description": description,
            "constraints": {
                "max_latency_ms": 120000,
                "required_capabilities": ["contract_review"],
            },
            "budget": {
                "max_amount": str(max_budget),
                "currency": "USD",
            },
            "success_criteria": {
                "min_confidence": 0.7,
            },
            "metadata": {
                "document_pages": document_pages,
                "source": "demo-ui",
            },
        }
        with httpx.Client(timeout=10.0) as client:
            resp = client.post(f"{AEX_WORK_PUBLISHER_URL}/v1/work", json=payload)
            if resp.status_code in [200, 201]:
                return resp.json()
            print(f"Work Publisher returned {resp.status_code}: {resp.text}")
    except Exception as e:
        print(f"Error publishing work to AEX: {e}")
    return {}


def create_contract_in_aex(work_id: str, provider_id: str, bid_id: str, agreed_price: float, a2a_endpoint: str) -> dict:
    """Create a contract via AEX Contract Engine."""
    try:
        payload = {
            "work_id": work_id,
            "provider_id": provider_id,
            "bid_id": bid_id,
            "agreed_price": str(agreed_price),
            "currency": "USD",
            "provider_endpoint": a2a_endpoint,
        }
        with httpx.Client(timeout=10.0) as client:
            resp = client.post(f"{AEX_CONTRACT_ENGINE_URL}/v1/contracts", json=payload)
            if resp.status_code in [200, 201]:
                return resp.json()
            print(f"Contract Engine returned {resp.status_code}: {resp.text}")
    except Exception as e:
        print(f"Error creating contract in AEX: {e}")
    return {}


def complete_contract_in_aex(contract_id: str, success: bool, result_summary: str, execution_time_ms: int) -> dict:
    """Mark contract as completed in AEX Contract Engine."""
    try:
        payload = {
            "success": success,
            "outcome_report": {
                "result_summary": result_summary[:500] if result_summary else "Work completed",
                "execution_time_ms": execution_time_ms,
            },
        }
        with httpx.Client(timeout=10.0) as client:
            resp = client.post(f"{AEX_CONTRACT_ENGINE_URL}/v1/contracts/{contract_id}/complete", json=payload)
            if resp.status_code in [200, 201]:
                return resp.json()
            print(f"Contract complete returned {resp.status_code}: {resp.text}")
    except Exception as e:
        print(f"Error completing contract in AEX: {e}")
    return {}


def detect_work_category(description: str) -> str:
    """Detect work category from description for payment provider selection."""
    desc = description.lower()

    # Contract keywords
    if any(kw in desc for kw in ["contract", "nda", "agreement", "terms", "clause"]):
        return "contracts"

    # Compliance keywords
    if any(kw in desc for kw in ["compliance", "regulatory", "audit", "gdpr", "hipaa"]):
        return "compliance"

    # IP/Patent keywords
    if any(kw in desc for kw in ["patent", "trademark", "copyright", "intellectual property"]):
        return "ip_patent"

    # Real estate keywords
    if any(kw in desc for kw in ["real estate", "property", "lease", "mortgage"]):
        return "real_estate"

    return "legal_research"


def fetch_payment_provider_bids(amount: float, work_category: str) -> list[dict]:
    """Fetch bids from payment providers via A2A protocol."""
    payment_providers = [
        {"id": "legalpay", "name": "LegalPay", "url": LEGALPAY_URL},
        {"id": "contractpay", "name": "ContractPay", "url": CONTRACTPAY_URL},
        {"id": "compliancepay", "name": "CompliancePay", "url": COMPLIANCEPAY_URL},
    ]

    bids = []
    for provider in payment_providers:
        try:
            bid_request = {
                "action": "bid",
                "amount": amount,
                "work_category": work_category,
                "currency": "USD",
            }
            a2a_payload = {
                "jsonrpc": "2.0",
                "method": "message/send",
                "id": f"payment-bid-{provider['id']}-{int(time.time())}",
                "params": {
                    "message": {
                        "role": "user",
                        "parts": [{"type": "text", "text": json.dumps(bid_request)}],
                    }
                },
            }

            with httpx.Client(timeout=10.0) as client:
                resp = client.post(f"{provider['url']}/a2a", json=a2a_payload)
                if resp.status_code == 200:
                    data = resp.json()
                    # Extract bid from A2A response
                    result = data.get("result", {})
                    history = result.get("history", [])
                    for msg in history:
                        if msg.get("role") == "agent":
                            for part in msg.get("parts", []):
                                if part.get("type") == "text":
                                    try:
                                        bid_resp = json.loads(part.get("text", "{}"))
                                        if bid_resp.get("action") == "bid_response":
                                            bid_data = bid_resp.get("bid", {})
                                            bids.append({
                                                "provider_id": bid_data.get("provider_id", provider["id"]),
                                                "provider_name": bid_data.get("provider_name", provider["name"]),
                                                "base_fee_percent": bid_data.get("base_fee_percent", 2.0),
                                                "reward_percent": bid_data.get("reward_percent", 1.0),
                                                "net_fee_percent": bid_data.get("net_fee_percent", 1.0),
                                                "processing_time_seconds": bid_data.get("processing_time_seconds", 5),
                                                "fraud_protection": bid_data.get("fraud_protection", "basic"),
                                            })
                                    except json.JSONDecodeError:
                                        pass
        except Exception as e:
            print(f"Error fetching bid from {provider['name']}: {e}")

    # If no bids received, return simulated bids
    if not bids:
        bids = simulate_payment_bids(amount, work_category)

    return bids


def simulate_payment_bids(amount: float, work_category: str) -> list[dict]:
    """Simulate payment provider bids for demo when agents unavailable."""
    # LegalPay - General (consistent 1% net)
    legalpay_reward = 1.0
    legalpay_net = 2.0 - legalpay_reward

    # ContractPay - Contract specialist (3% reward on contracts = -0.5% net!)
    contractpay_rewards = {"contracts": 3.0, "contract_review": 3.0, "real_estate": 2.5, "default": 1.0}
    contractpay_reward = contractpay_rewards.get(work_category, contractpay_rewards["default"])
    contractpay_net = 2.5 - contractpay_reward

    # CompliancePay - Compliance specialist (4% reward on compliance = -1% net!)
    compliancepay_rewards = {"compliance": 4.0, "compliance_check": 4.0, "ip_patent": 3.5, "default": 1.0}
    compliancepay_reward = compliancepay_rewards.get(work_category, compliancepay_rewards["default"])
    compliancepay_net = 3.0 - compliancepay_reward

    return [
        {
            "provider_id": "legalpay",
            "provider_name": "LegalPay",
            "base_fee_percent": 2.0,
            "reward_percent": legalpay_reward,
            "net_fee_percent": legalpay_net,
            "processing_time_seconds": 3,
            "fraud_protection": "basic",
        },
        {
            "provider_id": "contractpay",
            "provider_name": "ContractPay",
            "base_fee_percent": 2.5,
            "reward_percent": contractpay_reward,
            "net_fee_percent": contractpay_net,
            "processing_time_seconds": 5,
            "fraud_protection": "standard",
        },
        {
            "provider_id": "compliancepay",
            "provider_name": "CompliancePay",
            "base_fee_percent": 3.0,
            "reward_percent": compliancepay_reward,
            "net_fee_percent": compliancepay_net,
            "processing_time_seconds": 8,
            "fraud_protection": "advanced",
        },
    ]


def select_best_payment_provider(bids: list[dict]) -> dict:
    """Select the best payment provider based on lowest net fee."""
    if not bids:
        return {}
    return min(bids, key=lambda b: b.get("net_fee_percent", 100))


def process_settlement_via_aex(contract_id: str, consumer_id: str, provider_id: str,
                                agreed_price: float, use_ap2: bool = True) -> dict:
    """Process settlement through AEX Settlement service with AP2."""
    try:
        payload = {
            "contract_id": contract_id,
            "work_id": f"work-{contract_id}",
            "agent_id": provider_id,
            "consumer_id": consumer_id,
            "provider_id": provider_id,
            "domain": "legal",
            "started_at": datetime.utcnow().isoformat() + "Z",
            "completed_at": datetime.utcnow().isoformat() + "Z",
            "success": True,
            "agreed_price": str(agreed_price),
            "currency": "USD",
            "use_ap2": use_ap2,
            "payment_method": "card" if use_ap2 else "",
        }
        with httpx.Client(timeout=30.0) as client:
            resp = client.post(
                f"{AEX_SETTLEMENT_URL}/v1/contracts/complete",
                json=payload
            )
            if resp.status_code in [200, 201]:
                return resp.json()
    except Exception as e:
        print(f"Error processing settlement: {e}")
    return {}


def fetch_real_bids(pages: int) -> list[dict]:
    """Fetch real bids from the three legal agents via A2A protocol."""
    import sys
    print(f"[fetch_real_bids] Starting bid fetch for {pages} pages", file=sys.stderr, flush=True)

    legal_agents = [
        {"id": "legal-agent-a", "name": "Budget Legal AI", "url": LEGAL_AGENT_A_URL},
        {"id": "legal-agent-b", "name": "Standard Legal AI", "url": LEGAL_AGENT_B_URL},
        {"id": "legal-agent-c", "name": "Premium Legal AI", "url": LEGAL_AGENT_C_URL},
    ]

    bids = []
    for agent in legal_agents:
        try:
            bid_request = {
                "action": "get_bid",
                "document_pages": pages,
            }
            a2a_payload = {
                "jsonrpc": "2.0",
                "method": "message/send",
                "id": f"bid-{agent['id']}-{int(time.time())}",
                "params": {
                    "message": {
                        "role": "user",
                        "parts": [{"type": "text", "text": json.dumps(bid_request)}],
                    }
                },
            }

            with httpx.Client(timeout=10.0) as client:
                resp = client.post(f"{agent['url']}/a2a", json=a2a_payload)
                if resp.status_code == 200:
                    data = resp.json()
                    # Extract bid from A2A response
                    result = data.get("result", {})
                    history = result.get("history", [])
                    for msg in history:
                        if msg.get("role") == "agent":
                            for part in msg.get("parts", []):
                                if part.get("type") == "text":
                                    try:
                                        bid_resp = json.loads(part.get("text", "{}"))
                                        if bid_resp.get("action") == "bid_response":
                                            bid_data = bid_resp.get("bid", {})
                                            bids.append({
                                                "provider_id": bid_data.get("provider_id", agent["id"]),
                                                "provider_name": bid_data.get("provider_name", agent["name"]),
                                                "price": bid_data.get("price", 0.0),
                                                "confidence": bid_data.get("confidence", 0.85),
                                                "estimated_minutes": bid_data.get("estimated_minutes", 10),
                                                "trust_score": bid_data.get("trust_score", 0.85),
                                                "tier": bid_data.get("tier", "VERIFIED"),
                                                "a2a_endpoint": f"{agent['url']}/a2a",
                                            })
                                    except json.JSONDecodeError:
                                        pass
        except Exception as e:
            print(f"[fetch_real_bids] Error fetching bid from {agent['name']}: {e}", file=sys.stderr, flush=True)

    print(f"[fetch_real_bids] Collected {len(bids)} bids", file=sys.stderr, flush=True)

    # If no real bids received, fall back to simulated
    if not bids:
        print("[fetch_real_bids] No real bids, falling back to simulated", file=sys.stderr, flush=True)
        bids = simulate_bids(pages)

    return bids


def simulate_bids(pages: int) -> list[dict]:
    """Simulate bids from the three legal agents (fallback when agents unavailable).

    NOTE: These are SIMULATED/MOCK values - clearly different from real agent bids.
    Real bids come from fetch_real_bids() which calls agents via A2A.
    """
    # Use obviously different prices to distinguish from real bids
    # Real prices: Budget=$15, Standard=$17.50, Premium=$31 (for 5 pages)
    # Simulated: Use $99.xx to make it obvious these are mock values
    budget_price = 99.01 + pages
    standard_price = 99.02 + pages
    premium_price = 99.03 + pages

    return [
        {
            "provider_id": "budget-legal-ai",
            "provider_name": "Budget Legal AI (SIMULATED)",
            "price": budget_price,
            "confidence": 0.50,  # Obviously different from real 0.75
            "estimated_minutes": 999,
            "trust_score": 0.10,  # Obviously different from real 0.72
            "tier": "UNVERIFIED",  # Obviously different from real VERIFIED
            "a2a_endpoint": f"{LEGAL_AGENT_A_URL}/a2a",
        },
        {
            "provider_id": "standard-legal-ai",
            "provider_name": "Standard Legal AI (SIMULATED)",
            "price": standard_price,
            "confidence": 0.50,
            "estimated_minutes": 999,
            "trust_score": 0.10,
            "tier": "UNVERIFIED",
            "a2a_endpoint": f"{LEGAL_AGENT_B_URL}/a2a",
        },
        {
            "provider_id": "premium-legal-ai",
            "provider_name": "Premium Legal AI (SIMULATED)",
            "price": premium_price,
            "confidence": 0.50,
            "estimated_minutes": 999,
            "trust_score": 0.10,
            "tier": "UNVERIFIED",
            "a2a_endpoint": f"{LEGAL_AGENT_C_URL}/a2a",
        },
    ]


def evaluate_bids(bids: list[dict], strategy: str) -> list[dict]:
    """Evaluate and score bids based on strategy."""
    weights = {
        "lowest_price": {"price": 0.6, "trust": 0.2, "confidence": 0.1, "sla": 0.1},
        "best_quality": {"price": 0.1, "trust": 0.4, "confidence": 0.3, "sla": 0.2},
        "balanced": {"price": 0.3, "trust": 0.3, "confidence": 0.25, "sla": 0.15},
    }
    w = weights.get(strategy, weights["balanced"])

    max_price = max(b["price"] for b in bids)
    min_price = min(b["price"] for b in bids)
    price_range = max_price - min_price if max_price != min_price else 1

    for bid in bids:
        price_score = 1 - ((bid["price"] - min_price) / price_range)
        sla_score = min(1.0, 30 / max(bid["estimated_minutes"], 1))
        bid["score"] = (
            w["price"] * price_score +
            w["trust"] * bid["trust_score"] +
            w["confidence"] * bid["confidence"] +
            w["sla"] * sla_score
        )

    return sorted(bids, key=lambda x: x["score"], reverse=True)


def call_agent_a2a(url: str, message: str) -> tuple[str, int]:
    """Call an agent via A2A protocol."""
    a2a_url = f"{url}/a2a"
    payload = {
        "jsonrpc": "2.0",
        "method": "message/send",
        "id": f"demo-{int(time.time())}",
        "params": {
            "message": {
                "role": "user",
                "parts": [{"type": "text", "text": message}],
            }
        },
    }

    start = time.time()
    try:
        with httpx.Client(timeout=120.0) as client:
            resp = client.post(a2a_url, json=payload)
            resp.raise_for_status()
            data = resp.json()
            elapsed_ms = int((time.time() - start) * 1000)

            if "error" in data:
                return f"Error: {data['error'].get('message', 'Unknown')}", elapsed_ms

            result = data.get("result", {})
            history = result.get("history", [])

            for msg in reversed(history):
                if msg.get("role") == "agent":
                    for part in msg.get("parts", []):
                        if part.get("type") == "text":
                            return part.get("text", "No response"), elapsed_ms

            return "No response from agent", elapsed_ms
    except Exception as e:
        elapsed_ms = int((time.time() - start) * 1000)
        return f"Error: {str(e)}", elapsed_ms


# Event handlers
def on_work_change(e: me.InputEvent):
    import sys
    state = me.state(State)
    state.work_description = e.value
    print(f"[on_work_change] Text updated: '{e.value[:30] if e.value else ''}...'", file=sys.stderr, flush=True)


def on_pages_change(e: me.InputEvent):
    state = me.state(State)
    try:
        state.document_pages = int(e.value)
    except:
        state.document_pages = 5


def on_strategy_change(e: me.SelectSelectionChangeEvent):
    state = me.state(State)
    state.bid_strategy = e.value


def add_log(state, message: str):
    """Add a message to the output log."""
    timestamp = datetime.now().strftime("%H:%M:%S")
    state.log_messages = state.log_messages + [f"[{timestamp}] {message}"]


def on_run_workflow(e: me.ClickEvent):
    """Run the entire AEX workflow automatically and log all steps."""
    state = me.state(State)

    if not state.work_description.strip():
        state.error = "Please describe your legal task"
        return

    # Reset state for new run
    state.error = ""
    state.log_messages = []
    state.is_running = True
    state.work_id = f"work-{int(time.time())}"

    # Create task result immediately (will update as we progress)
    current_task = TaskResult(
        task_id=state.work_id,
        description=state.work_description,
        document_pages=state.document_pages,
        bid_strategy=state.bid_strategy,
        status="pending",
        current_step=0,
        timestamp=datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
    )
    # Add to front of results list immediately so user sees it
    state.task_results = [current_task] + state.task_results

    add_log(state, f"=== AEX Workflow Started ===")
    add_log(state, f"Work ID: {state.work_id}")
    add_log(state, f"Task: {state.work_description[:100]}...")
    add_log(state, f"Document pages: {state.document_pages}")
    add_log(state, f"Bid strategy: {state.bid_strategy}")
    add_log(state, "")

    # STEP 1: Fetch bids from legal agents
    current_task.status = "bidding"
    current_task.current_step = 1
    state.task_results = [current_task] + state.task_results[1:]  # Update in list

    add_log(state, "[STEP 1/7] COLLECTING BIDS from Legal Agents...")
    raw_bids = fetch_real_bids(state.document_pages)
    add_log(state, f"  Received {len(raw_bids)} bids:")

    for b in raw_bids:
        add_log(state, f"    - {b['provider_name']}: ${b['price']:.2f} | {b['tier']} | trust={b['trust_score']:.2f}")

    current_task.bids = [
        {
            "provider_name": b["provider_name"],
            "price": b["price"],
            "tier": b["tier"],
            "trust_score": b["trust_score"],
            "score": 0.0,
        }
        for b in raw_bids
    ]
    state.task_results = [current_task] + state.task_results[1:]
    add_log(state, "")

    # STEP 2: Evaluate bids
    current_task.status = "evaluating"
    current_task.current_step = 2
    state.task_results = [current_task] + state.task_results[1:]

    add_log(state, f"[STEP 2/7] EVALUATING BIDS using '{state.bid_strategy}' strategy...")
    bid_dicts = [
        {
            "provider_id": b.get("provider_id", b["provider_name"].lower().replace(" ", "-")),
            "provider_name": b["provider_name"],
            "price": b["price"],
            "confidence": 0.85,
            "estimated_minutes": 10,
            "trust_score": b["trust_score"],
            "tier": b["tier"],
            "a2a_endpoint": "",
        }
        for b in current_task.bids
    ]
    evaluated = evaluate_bids(bid_dicts, state.bid_strategy)

    add_log(state, "  Scores:")
    for i, b in enumerate(evaluated):
        marker = " << WINNER" if i == 0 else ""
        add_log(state, f"    {i+1}. {b['provider_name']}: score={b['score']:.3f}{marker}")

    # Update bids with scores
    current_task.bids = [
        {
            "provider_name": b["provider_name"],
            "price": b["price"],
            "tier": b["tier"],
            "trust_score": b["trust_score"],
            "score": round(b["score"], 3),
        }
        for b in evaluated
    ]
    state.task_results = [current_task] + state.task_results[1:]
    add_log(state, "")

    # STEP 3: Award contract to winner
    winner = evaluated[0]
    current_task.status = "awarded"
    current_task.current_step = 3
    current_task.winner_name = winner["provider_name"]
    current_task.winner_tier = winner["tier"]
    current_task.winner_price = winner["price"]
    current_task.winner_score = round(winner["score"], 3)
    current_task.contract_id = f"contract-{int(time.time())}"
    state.task_results = [current_task] + state.task_results[1:]

    # Also update state for backward compatibility
    state.winner_provider_id = winner.get("provider_id", winner["provider_name"].lower().replace(" ", "-"))
    state.winner_provider_name = winner["provider_name"]
    state.winner_tier = winner["tier"]
    state.agreed_price = winner["price"]
    state.contract_id = current_task.contract_id

    add_log(state, f"[STEP 3/7] CONTRACT AWARDED")
    add_log(state, f"  Contract ID: {current_task.contract_id}")
    add_log(state, f"  Winner: {current_task.winner_name}")
    add_log(state, f"  Tier: {current_task.winner_tier}")
    add_log(state, f"  Agreed Price: ${current_task.winner_price:.2f}")
    add_log(state, "")

    # STEP 4: Execute work via A2A
    current_task.status = "executing"
    current_task.current_step = 4
    state.task_results = [current_task] + state.task_results[1:]

    add_log(state, f"[STEP 4/7] EXECUTING WORK via A2A Protocol...")
    url = PROVIDER_URL_MAP.get(state.winner_provider_id, LEGAL_AGENT_A_URL)
    add_log(state, f"  Calling {current_task.winner_name} at {url}...")

    response_text, elapsed_ms = call_agent_a2a(url, state.work_description)
    current_task.agent_response = response_text
    current_task.execution_time_ms = elapsed_ms
    state.agent_response = response_text
    state.execution_time_ms = elapsed_ms
    state.task_results = [current_task] + state.task_results[1:]

    add_log(state, f"  Response received in {elapsed_ms}ms")
    add_log(state, f"  Response length: {len(response_text)} characters")
    add_log(state, "")

    # STEP 5: AP2 Payment Provider Selection
    current_task.status = "paying"
    current_task.current_step = 5
    state.task_results = [current_task] + state.task_results[1:]

    add_log(state, f"[STEP 5/7] AP2 PAYMENT - Selecting Payment Provider...")

    # Determine work category for payment rewards
    work_category = "contracts"  # Default, could be inferred from task
    if "compliance" in state.work_description.lower():
        work_category = "compliance"
    elif "patent" in state.work_description.lower() or "ip" in state.work_description.lower():
        work_category = "ip_patent"
    elif "real estate" in state.work_description.lower():
        work_category = "real_estate"

    add_log(state, f"  Work category: {work_category}")
    add_log(state, f"  Fetching payment provider bids for ${current_task.winner_price:.2f}...")

    # Fetch payment provider bids
    payment_bids = fetch_payment_provider_bids(current_task.winner_price, work_category)
    add_log(state, f"  Received {len(payment_bids)} payment provider bids:")
    for pb in payment_bids:
        net_fee = pb.get("net_fee_percent", 0)
        net_fee_str = f"{net_fee:.1f}% fee" if net_fee >= 0 else f"{abs(net_fee):.1f}% CASHBACK"
        add_log(state, f"    - {pb['provider_name']}: {pb.get('base_fee_percent', 0):.1f}% base, {pb.get('reward_percent', 0):.1f}% reward = {net_fee_str}")

    # Select best payment provider (lowest net fee)
    best_payment = select_best_payment_provider(payment_bids)
    current_task.ap2_payment_provider = best_payment.get("provider_name", "LegalPay")
    current_task.ap2_payment_method = "card"
    current_task.ap2_cart_mandate_id = f"cart-{int(time.time())}"
    state.task_results = [current_task] + state.task_results[1:]

    net_fee_pct = best_payment.get("net_fee_percent", 1.0)
    add_log(state, f"  Selected: {current_task.ap2_payment_provider}")
    if net_fee_pct < 0:
        add_log(state, f"  YOU EARN {abs(net_fee_pct):.1f}% CASHBACK on this transaction!")
    else:
        add_log(state, f"  Net fee: {net_fee_pct:.1f}%")
    add_log(state, f"  Cart Mandate ID: {current_task.ap2_cart_mandate_id}")
    add_log(state, "")

    # STEP 6: Process Payment
    current_task.current_step = 6
    state.task_results = [current_task] + state.task_results[1:]

    add_log(state, f"[STEP 6/7] AP2 PAYMENT - Processing...")
    add_log(state, f"  Creating Payment Mandate...")
    add_log(state, f"  Amount: ${current_task.winner_price:.2f}")

    # Calculate actual payment amounts
    base_fee_pct = best_payment.get("base_fee_percent", 2.0)
    reward_pct = best_payment.get("reward_percent", 1.0)
    base_fee_amount = round(current_task.winner_price * base_fee_pct / 100, 2)
    reward_amount = round(current_task.winner_price * reward_pct / 100, 2)
    net_fee_amount = round(base_fee_amount - reward_amount, 2)

    add_log(state, f"  Base fee ({base_fee_pct}%): ${base_fee_amount:.2f}")
    add_log(state, f"  Reward ({reward_pct}%): -${reward_amount:.2f}")
    if net_fee_amount < 0:
        add_log(state, f"  Net: +${abs(net_fee_amount):.2f} CASHBACK!")
    else:
        add_log(state, f"  Net fee: ${net_fee_amount:.2f}")

    current_task.ap2_payment_receipt_id = f"receipt-{int(time.time())}"
    add_log(state, f"  Payment Receipt ID: {current_task.ap2_payment_receipt_id}")
    add_log(state, f"  Payment Status: COMPLETED")
    add_log(state, "")

    # STEP 7: Settlement
    current_task.status = "settling"
    current_task.current_step = 7
    state.task_results = [current_task] + state.task_results[1:]

    state.platform_fee = round(current_task.winner_price * 0.10, 2)  # 10% platform fee
    current_task.platform_fee = state.platform_fee
    current_task.provider_payout = round(current_task.winner_price - state.platform_fee, 2)
    state.provider_payout = current_task.provider_payout
    state.settlement_timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    current_task.timestamp = state.settlement_timestamp

    add_log(state, f"[STEP 7/7] SETTLEMENT")
    add_log(state, f"  Agreed Price: ${current_task.winner_price:.2f}")
    add_log(state, f"  Platform Fee (10%): ${current_task.platform_fee:.2f}")
    add_log(state, f"  Provider Payout: ${current_task.provider_payout:.2f}")
    add_log(state, f"  Settlement Time: {state.settlement_timestamp}")
    add_log(state, "")

    # Mark as completed
    current_task.status = "completed"
    state.task_results = [current_task] + state.task_results[1:]

    # Update stats
    state.total_transactions += 1
    state.successful_transactions += 1
    state.platform_revenue += state.platform_fee
    state.total_response_time += elapsed_ms
    state.avg_response_time = state.total_response_time // state.total_transactions

    add_log(state, f"=== WORKFLOW COMPLETE ===")
    add_log(state, f"Total transactions: {state.total_transactions}")
    add_log(state, f"Platform revenue: ${state.platform_revenue:.2f}")

    state.is_running = False
    state.work_description = ""  # Clear for next task


def on_submit_work(e: me.ClickEvent):
    """Legacy submit - redirects to on_run_workflow."""
    on_run_workflow(e)


def on_simulate_bidding(e: me.ClickEvent):
    """Fetch real bids from legal agents via A2A protocol."""
    import sys
    print(f"[on_simulate_bidding] Called - fetching real bids", file=sys.stderr, flush=True)
    state = me.state(State)
    # Try to fetch real bids from agents, falls back to simulation if unavailable
    raw_bids = fetch_real_bids(state.document_pages)
    print(f"[on_simulate_bidding] Got {len(raw_bids)} bids", file=sys.stderr, flush=True)
    state.bids = [
        Bid(
            provider_id=b["provider_id"],
            provider_name=b["provider_name"],
            price=round(b["price"], 2),
            confidence=round(b["confidence"], 2),
            estimated_minutes=b["estimated_minutes"],
            trust_score=b["trust_score"],
            tier=b["tier"],
            score=0.0,
            a2a_endpoint=b.get("a2a_endpoint", ""),
        )
        for b in raw_bids
    ]
    state.bidding_complete = True


def on_evaluate_bids(e: me.ClickEvent):
    state = me.state(State)
    # Build a map of provider_id to a2a_endpoint before evaluation
    endpoint_map = {b.provider_id: b.a2a_endpoint for b in state.bids}
    bid_dicts = [
        {
            "provider_id": b.provider_id,
            "provider_name": b.provider_name,
            "price": b.price,
            "confidence": b.confidence,
            "estimated_minutes": b.estimated_minutes,
            "trust_score": b.trust_score,
            "tier": b.tier,
            "a2a_endpoint": b.a2a_endpoint,
        }
        for b in state.bids
    ]
    evaluated = evaluate_bids(bid_dicts, state.bid_strategy)
    state.bids = [
        Bid(
            provider_id=b["provider_id"],
            provider_name=b["provider_name"],
            price=b["price"],
            confidence=round(b["confidence"], 2),
            estimated_minutes=b["estimated_minutes"],
            trust_score=b["trust_score"],
            tier=b["tier"],
            score=round(b["score"], 3),
            a2a_endpoint=b.get("a2a_endpoint", endpoint_map.get(b["provider_id"], "")),
        )
        for b in evaluated
    ]
    state.selected_bid_index = 0  # Reset to top scorer when evaluating
    state.step = 3


def on_select_bid(e: me.ClickEvent):
    """Manual override for winner selection."""
    state = me.state(State)
    index = int(e.key)
    state.selected_bid_index = index


def on_award_contract(e: me.ClickEvent):
    state = me.state(State)
    if state.bids:
        # Use manually selected bid (defaults to top scorer at index 0)
        winner = state.bids[state.selected_bid_index]
        state.winner_provider_id = winner.provider_id
        state.winner_provider_name = winner.provider_name
        state.winner_tier = winner.tier
        state.winner_a2a_endpoint = winner.a2a_endpoint
        state.agreed_price = winner.price

        # Try to create contract in AEX Contract Engine
        contract_result = create_contract_in_aex(
            work_id=state.work_id,
            provider_id=winner.provider_id,
            bid_id=f"bid-{winner.provider_id}-{int(time.time())}",
            agreed_price=winner.price,
            a2a_endpoint=winner.a2a_endpoint,
        )

        if contract_result and contract_result.get("contract_id"):
            state.contract_id = contract_result["contract_id"]
        else:
            # Fallback to local contract ID
            state.contract_id = f"contract-{int(time.time())}"

        state.step = 4


def on_execute_work(e: me.ClickEvent):
    state = me.state(State)
    state.loading = True
    state.error = ""

    # Use the A2A endpoint from the winning bid, or fall back to URL map
    if state.winner_a2a_endpoint:
        # Extract base URL from A2A endpoint (remove /a2a suffix)
        url = state.winner_a2a_endpoint.replace("/a2a", "")
    else:
        url = PROVIDER_URL_MAP.get(state.winner_provider_id, LEGAL_AGENT_A_URL)

    response, elapsed = call_agent_a2a(url, state.work_description)

    state.agent_response = response
    state.execution_time_ms = elapsed

    # Mark contract as completed in AEX (if contract was created there)
    success = not response.startswith("Error:")
    complete_contract_in_aex(
        contract_id=state.contract_id,
        success=success,
        result_summary=response,
        execution_time_ms=elapsed,
    )

    state.loading = False
    state.step = 5


def on_fetch_payment_bids(e: me.ClickEvent):
    """Fetch bids from payment providers."""
    state = me.state(State)

    # Detect work category from description
    state.work_category = detect_work_category(state.work_description)

    # Fetch payment provider bids
    state.payment_provider_bids = fetch_payment_provider_bids(state.agreed_price, state.work_category)

    # Select the best payment provider (lowest net fee)
    state.selected_payment_provider = select_best_payment_provider(state.payment_provider_bids)

    if state.selected_payment_provider:
        state.payment_base_fee = state.selected_payment_provider.get("base_fee_percent", 2.0)
        state.payment_reward = state.selected_payment_provider.get("reward_percent", 1.0)
        state.payment_net_cost = state.selected_payment_provider.get("net_fee_percent", 1.0)

    state.step = 6


def on_settle(e: me.ClickEvent):
    state = me.state(State)

    # Try to process settlement via AEX with AP2
    settlement_result = process_settlement_via_aex(
        contract_id=state.contract_id,
        consumer_id="consumer-demo-001",
        provider_id=state.winner_provider_id,
        agreed_price=state.agreed_price,
        use_ap2=True,
    )

    if settlement_result:
        # Use real settlement data
        state.platform_fee = float(settlement_result.get("platform_fee", state.agreed_price * 0.15))
        state.provider_payout = float(settlement_result.get("provider_payout", state.agreed_price * 0.85))
        state.ap2_enabled = settlement_result.get("ap2_enabled", False)
        state.payment_mandate_id = settlement_result.get("payment_mandate_id", "")
        state.payment_receipt_id = settlement_result.get("payment_receipt_id", "")
        state.payment_transaction_id = settlement_result.get("payment_transaction_id", "")
        state.payment_method = settlement_result.get("payment_method", "")
    else:
        # Fallback to local calculation
        state.platform_fee = round(state.agreed_price * 0.15, 2)
        state.provider_payout = round(state.agreed_price - state.platform_fee, 2)
        # Simulate AP2 for demo purposes
        state.ap2_enabled = True
        state.payment_mandate_id = f"pm_{state.contract_id}"
        state.payment_receipt_id = f"rcpt_{int(time.time())}"
        state.payment_transaction_id = f"txn_{int(time.time())}"
        state.payment_method = "Demo Visa ****1234"

    state.settlement_timestamp = datetime.utcnow().strftime("%Y-%m-%d %H:%M:%S UTC")

    # Update all platform KPIs dynamically
    state.platform_revenue = round(state.platform_revenue + state.platform_fee, 2)
    state.total_transactions += 1
    state.successful_transactions += 1
    state.total_response_time += state.execution_time_ms
    state.avg_response_time = state.total_response_time // state.total_transactions

    state.step = 7


def on_restart(e: me.ClickEvent):
    state = me.state(State)
    state.step = 1
    state.work_description = ""
    state.document_pages = 5
    state.bid_strategy = "balanced"
    state.work_id = ""
    state.bids = []
    state.bidding_complete = False
    state.selected_bid_index = 0
    state.winner_provider_id = ""
    state.winner_provider_name = ""
    state.winner_tier = ""
    state.winner_a2a_endpoint = ""
    state.contract_id = ""
    state.agent_response = ""
    state.execution_time_ms = 0
    state.agreed_price = 0.0
    state.platform_fee = 0.0
    state.provider_payout = 0.0
    state.settlement_timestamp = ""
    state.loading = False
    state.error = ""
    # Reset AP2 fields
    state.ap2_enabled = False
    state.payment_mandate_id = ""
    state.payment_receipt_id = ""
    state.payment_transaction_id = ""
    state.payment_method = ""
    # Reset payment provider fields
    state.payment_provider_bids = []
    state.selected_payment_provider = {}
    state.payment_base_fee = 0.0
    state.payment_reward = 0.0
    state.payment_net_cost = 0.0
    state.work_category = ""


# Dark theme styles
DARK_BG = "#0a0f1a"
CARD_BG = "#131b2e"
CARD_BORDER = "#1e2a45"
ACCENT_BLUE = "#3b82f6"
ACCENT_GREEN = "#10b981"
ACCENT_ORANGE = "#f59e0b"
ACCENT_PURPLE = "#8b5cf6"
ACCENT_CYAN = "#06b6d4"
TEXT_PRIMARY = "#ffffff"
TEXT_SECONDARY = "#94a3b8"


def render_kpi_card(value: str, label: str, color: str):
    """Render a KPI metric card."""
    with me.box(style=me.Style(
        background=CARD_BG,
        border_radius=8,
        padding=me.Padding.all(16),
        border=me.Border.all(me.BorderSide(width=1, color=CARD_BORDER)),
        text_align="center",
        flex_grow=1,
    )):
        me.text(value, style=me.Style(
            font_size=28,
            font_weight="bold",
            color=color,
        ))
        me.text(label, style=me.Style(
            font_size=12,
            color=TEXT_SECONDARY,
            margin=me.Margin(top=4),
        ))


def render_topology():
    """Render the visual workflow topology."""
    state = me.state(State)

    steps = [
        ("WORK", "Submit Task", ACCENT_BLUE, 1),
        ("BID", "Collect Bids", ACCENT_ORANGE, 2),
        ("EVAL", "Evaluate", ACCENT_PURPLE, 3),
        ("AWARD", "Contract", ACCENT_CYAN, 4),
        ("EXEC", "A2A Call", ACCENT_GREEN, 5),
        ("PAY", "Payment Bids", "#f97316", 6),
        ("SETTLE", "Settlement", "#ec4899", 7),
    ]

    with me.box(style=me.Style(
        background=CARD_BG,
        border_radius=12,
        padding=me.Padding.all(24),
        border=me.Border.all(me.BorderSide(width=1, color=CARD_BORDER)),
        margin=me.Margin(bottom=24),
    )):
        me.text("Agent Exchange Workflow", style=me.Style(
            font_size=16,
            font_weight="bold",
            color=TEXT_PRIMARY,
            margin=me.Margin(bottom=20),
        ))

        # Topology nodes
        with me.box(style=me.Style(
            display="flex",
            justify_content="space-between",
            align_items="center",
            flex_wrap="wrap",
            gap=8,
        )):
            for i, (code, label, color, step_num) in enumerate(steps):
                is_active = state.step == step_num
                is_complete = state.step > step_num

                node_color = color if (is_active or is_complete) else "#374151"
                opacity = "1" if (is_active or is_complete) else "0.5"

                with me.box(style=me.Style(
                    display="flex",
                    align_items="center",
                    gap=8,
                )):
                    # Node circle
                    with me.box(style=me.Style(
                        width=56,
                        height=56,
                        border_radius="50%",
                        background=f"linear-gradient(135deg, {node_color}, {node_color}88)",
                        display="flex",
                        flex_direction="column",
                        align_items="center",
                        justify_content="center",
                        box_shadow=f"0 0 20px {node_color}44" if is_active else "none",
                        border=me.Border.all(me.BorderSide(width=3, color=node_color if is_active else "transparent")),
                    )):
                        me.text("✓" if is_complete else code[:1], style=me.Style(
                            font_size=16,
                            font_weight="bold",
                            color=TEXT_PRIMARY,
                        ))

                    # Label below
                    with me.box(style=me.Style(width=60)):
                        me.text(label, style=me.Style(
                            font_size=10,
                            color=TEXT_PRIMARY if is_active else TEXT_SECONDARY,
                            text_align="center",
                        ))

                    # Connector line
                    if i < len(steps) - 1:
                        connector_color = ACCENT_GREEN if is_complete else "#374151"
                        me.text("―――→", style=me.Style(
                            color=connector_color,
                            font_size=14,
                            margin=me.Margin(left=4, right=4),
                        ))


def render_provider_card_from_dict(provider: dict):
    """Render a provider card from provider data dict."""
    name = provider.get("name", "Unknown Agent")
    tier = provider.get("trust_tier", "UNVERIFIED")
    trust = provider.get("trust_score", 0.5)
    status = provider.get("status", "UNKNOWN")
    capabilities = provider.get("capabilities", [])

    tier_colors = {
        "UNVERIFIED": "#6b7280",
        "VERIFIED": ACCENT_ORANGE,
        "TRUSTED": ACCENT_BLUE,
        "PREFERRED": ACCENT_GREEN,
    }
    status_colors = {
        "ACTIVE": ACCENT_GREEN,
        "PENDING_VERIFICATION": ACCENT_ORANGE,
        "SUSPENDED": "#ef4444",
        "INACTIVE": "#6b7280",
    }

    with me.box(style=me.Style(
        background=CARD_BG,
        border_radius=8,
        padding=me.Padding.all(16),
        border=me.Border.all(me.BorderSide(width=1, color=CARD_BORDER)),
        flex_grow=1,
        min_width=200,
    )):
        with me.box(style=me.Style(display="flex", justify_content="space-between", align_items="center")):
            me.text(name, style=me.Style(font_weight="bold", color=TEXT_PRIMARY, font_size=14))
            with me.box(style=me.Style(
                background=tier_colors.get(tier, "#666"),
                padding=me.Padding.symmetric(horizontal=8, vertical=2),
                border_radius=4,
            )):
                me.text(tier, style=me.Style(font_size=10, color=TEXT_PRIMARY))

        with me.box(style=me.Style(margin=me.Margin(top=12))):
            with me.box(style=me.Style(display="flex", justify_content="space-between")):
                me.text("Trust Score:", style=me.Style(font_size=12, color=TEXT_SECONDARY))
                me.text(f"{trust:.0%}", style=me.Style(font_size=12, color=ACCENT_GREEN))
            with me.box(style=me.Style(display="flex", justify_content="space-between", margin=me.Margin(top=4))):
                me.text("Status:", style=me.Style(font_size=12, color=TEXT_SECONDARY))
                me.text(status, style=me.Style(font_size=12, color=status_colors.get(status, "#6b7280")))

        # Show capabilities if available
        if capabilities:
            with me.box(style=me.Style(margin=me.Margin(top=8), display="flex", flex_wrap="wrap", gap=4)):
                for cap in capabilities[:3]:  # Show max 3 capabilities
                    with me.box(style=me.Style(
                        background="#1e293b",
                        padding=me.Padding.symmetric(horizontal=6, vertical=2),
                        border_radius=4,
                    )):
                        me.text(cap, style=me.Style(font_size=9, color=TEXT_SECONDARY))


def render_provider_card(name: str, tier: str, status: str, trust: float, color: str):
    """Render a provider card (legacy interface)."""
    render_provider_card_from_dict({
        "name": name,
        "trust_tier": tier,
        "trust_score": trust,
        "status": "ACTIVE" if status == "Online" else "INACTIVE",
    })


def render_step_content():
    """Render the current step's content."""
    state = me.state(State)

    with me.box(style=me.Style(
        background=CARD_BG,
        border_radius=12,
        padding=me.Padding.all(24),
        border=me.Border.all(me.BorderSide(width=1, color=CARD_BORDER)),
    )):
        if state.step == 1:
            render_step_1()
        elif state.step == 2:
            render_step_2()
        elif state.step == 3:
            render_step_3()
        elif state.step == 4:
            render_step_4()
        elif state.step == 5:
            render_step_5()
        elif state.step == 6:
            render_step_6()
        elif state.step == 7:
            render_step_7()


def render_step_1():
    """Step 1: Submit Work."""
    state = me.state(State)

    me.text("Submit Work Request", style=me.Style(
        font_size=18,
        font_weight="bold",
        color=TEXT_PRIMARY,
        margin=me.Margin(bottom=16),
    ))

    me.text("Describe your legal task:", style=me.Style(color=TEXT_SECONDARY, margin=me.Margin(bottom=8)))
    me.textarea(
        value=state.work_description,
        on_input=on_work_change,
        placeholder="e.g., Review this NDA for potential risks...",
        style=me.Style(
            width="100%",
            min_height=120,
            background="#1e293b",
            color=TEXT_PRIMARY,
            border_radius=8,
        ),
    )

    with me.box(style=me.Style(display="flex", gap=24, margin=me.Margin(top=16))):
        with me.box():
            me.text("Document pages:", style=me.Style(color=TEXT_SECONDARY, margin=me.Margin(bottom=8)))
            me.input(
                value=str(state.document_pages),
                on_input=on_pages_change,
                type="number",
                style=me.Style(width=80, background="#1e293b", color=TEXT_PRIMARY),
            )

        with me.box():
            me.text("Bid strategy:", style=me.Style(color=TEXT_SECONDARY, margin=me.Margin(bottom=8)))
            me.select(
                value=state.bid_strategy,
                options=[
                    me.SelectOption(label="Balanced", value="balanced"),
                    me.SelectOption(label="Lowest Price", value="lowest_price"),
                    me.SelectOption(label="Best Quality", value="best_quality"),
                ],
                on_selection_change=on_strategy_change,
            )

    if state.error:
        me.text(state.error, style=me.Style(color="#ef4444", margin=me.Margin(top=16)))

    me.button(
        "Submit to Marketplace →",
        on_click=on_submit_work,
        type="flat",
        style=me.Style(
            margin=me.Margin(top=24),
            background=ACCENT_BLUE,
            color=TEXT_PRIMARY,
            padding=me.Padding.symmetric(horizontal=24, vertical=12),
            border_radius=8,
        ),
    )


def render_step_2():
    """Step 2: Bidding."""
    state = me.state(State)

    me.text("Bidding Phase", style=me.Style(
        font_size=18,
        font_weight="bold",
        color=TEXT_PRIMARY,
        margin=me.Margin(bottom=8),
    ))
    me.text(f"Work ID: {state.work_id}", style=me.Style(
        font_family="monospace",
        color=TEXT_SECONDARY,
        margin=me.Margin(bottom=16),
    ))

    if not state.bidding_complete:
        me.button(
            "Open Bidding Window",
            on_click=on_simulate_bidding,
            style=me.Style(background=ACCENT_ORANGE, color=TEXT_PRIMARY, padding=me.Padding.symmetric(horizontal=24, vertical=12), border_radius=8),
        )
    else:
        me.text("Bids Received:", style=me.Style(color=TEXT_SECONDARY, margin=me.Margin(bottom=12)))

        for bid in state.bids:
            tier_colors = {"VERIFIED": ACCENT_ORANGE, "TRUSTED": ACCENT_BLUE, "PREFERRED": ACCENT_GREEN}
            with me.box(style=me.Style(
                background="#1e293b",
                padding=me.Padding.all(16),
                border_radius=8,
                margin=me.Margin(bottom=8),
                display="flex",
                justify_content="space-between",
                align_items="center",
            )):
                with me.box():
                    me.text(bid.provider_name, style=me.Style(font_weight="bold", color=TEXT_PRIMARY))
                    with me.box(style=me.Style(
                        background=tier_colors.get(bid.tier, "#666"),
                        padding=me.Padding.symmetric(horizontal=6, vertical=2),
                        border_radius=4,
                        display="inline-block",
                        margin=me.Margin(top=4),
                    )):
                        me.text(bid.tier, style=me.Style(font_size=10, color=TEXT_PRIMARY))

                with me.box(style=me.Style(text_align="right")):
                    me.text(f"${bid.price:.2f}", style=me.Style(font_size=20, font_weight="bold", color=ACCENT_GREEN))
                    me.text(f"{bid.confidence:.0%} confidence", style=me.Style(font_size=12, color=TEXT_SECONDARY))

        me.button(
            "Evaluate Bids →",
            on_click=on_evaluate_bids,
            style=me.Style(margin=me.Margin(top=16), background=ACCENT_PURPLE, color=TEXT_PRIMARY, padding=me.Padding.symmetric(horizontal=24, vertical=12), border_radius=8),
        )


def render_step_3():
    """Step 3: Evaluation."""
    state = me.state(State)

    me.text("Bid Evaluation", style=me.Style(font_size=18, font_weight="bold", color=TEXT_PRIMARY, margin=me.Margin(bottom=8)))
    me.text(f"Strategy: {state.bid_strategy}", style=me.Style(color=TEXT_SECONDARY, margin=me.Margin(bottom=4)))
    me.text("Click any bid to select winner", style=me.Style(color=ACCENT_CYAN, font_size=12, margin=me.Margin(bottom=16)))

    for i, bid in enumerate(state.bids):
        is_selected = i == state.selected_bid_index
        with me.box(
            key=str(i),
            on_click=on_select_bid,
            style=me.Style(
                background=ACCENT_GREEN + "22" if is_selected else "#1e293b",
                padding=me.Padding.all(16),
                border_radius=8,
                margin=me.Margin(bottom=8),
                border=me.Border.all(me.BorderSide(width=2, color=ACCENT_GREEN if is_selected else "transparent")),
                cursor="pointer",
            ),
        ):
            with me.box(style=me.Style(display="flex", justify_content="space-between", align_items="center")):
                with me.box(style=me.Style(display="flex", align_items="center", gap=12)):
                    me.text(f"#{i+1}", style=me.Style(font_size=18, font_weight="bold", color=ACCENT_GREEN if is_selected else TEXT_SECONDARY))
                    me.text(bid.provider_name, style=me.Style(font_weight="bold", color=TEXT_PRIMARY))
                    if is_selected:
                        me.text("SELECTED", style=me.Style(color=ACCENT_GREEN, font_size=11, font_weight="bold"))

                me.text(f"Score: {bid.score:.3f}", style=me.Style(font_size=16, font_weight="bold", color=ACCENT_CYAN))

    me.button(
        "Award Contract →",
        on_click=on_award_contract,
        style=me.Style(margin=me.Margin(top=16), background=ACCENT_CYAN, color=TEXT_PRIMARY, padding=me.Padding.symmetric(horizontal=24, vertical=12), border_radius=8),
    )


def render_step_4():
    """Step 4: Award."""
    state = me.state(State)

    me.text("Contract Awarded", style=me.Style(font_size=18, font_weight="bold", color=ACCENT_GREEN, margin=me.Margin(bottom=16)))

    with me.box(style=me.Style(background="#1e293b", padding=me.Padding.all(20), border_radius=8, margin=me.Margin(bottom=16))):
        me.text(f"Contract: {state.contract_id}", style=me.Style(font_family="monospace", color=TEXT_SECONDARY, margin=me.Margin(bottom=8)))
        me.text(f"Winner: {state.winner_provider_name}", style=me.Style(font_size=16, font_weight="bold", color=TEXT_PRIMARY))
        me.text(f"Price: ${state.agreed_price:.2f}", style=me.Style(color=ACCENT_GREEN))

    me.text("Ready for direct A2A communication with provider.", style=me.Style(color=TEXT_SECONDARY, margin=me.Margin(bottom=16)))

    me.button(
        "Execute via A2A →" if not state.loading else "Executing...",
        on_click=on_execute_work,
        disabled=state.loading,
        style=me.Style(background=ACCENT_GREEN, color=TEXT_PRIMARY, padding=me.Padding.symmetric(horizontal=24, vertical=12), border_radius=8),
    )

    if state.loading:
        me.progress_spinner()


def render_step_5():
    """Step 5: Execution."""
    state = me.state(State)

    me.text("Execution Complete", style=me.Style(font_size=18, font_weight="bold", color=TEXT_PRIMARY, margin=me.Margin(bottom=8)))
    me.text(f"Response time: {state.execution_time_ms}ms", style=me.Style(color=ACCENT_GREEN, margin=me.Margin(bottom=16)))

    with me.box(style=me.Style(
        background="#1e293b",
        padding=me.Padding.all(20),
        border_radius=8,
        max_height=300,
        overflow_y="auto",
        margin=me.Margin(bottom=16),
        color=TEXT_PRIMARY,
    )):
        me.text(state.agent_response, style=me.Style(
            color=TEXT_PRIMARY,
            white_space="pre-wrap",
            font_size=14,
            line_height="1.6",
        ))

    me.button(
        "Select Payment Provider →",
        on_click=on_fetch_payment_bids,
        style=me.Style(background="#f97316", color=TEXT_PRIMARY, padding=me.Padding.symmetric(horizontal=24, vertical=12), border_radius=8),
    )


def render_step_6():
    """Step 6: Payment Provider Selection."""
    state = me.state(State)

    me.text("Payment Provider Marketplace", style=me.Style(font_size=18, font_weight="bold", color=TEXT_PRIMARY, margin=me.Margin(bottom=4)))
    me.text("Competing payment providers bid for this transaction", style=me.Style(font_size=12, color=TEXT_SECONDARY, margin=me.Margin(bottom=16)))

    # Work category detected
    with me.box(style=me.Style(
        background="#1e293b",
        border_radius=8,
        padding=me.Padding.all(12),
        margin=me.Margin(bottom=16),
        display="flex",
        justify_content="space-between",
        align_items="center",
    )):
        me.text("Work Category:", style=me.Style(color=TEXT_SECONDARY, font_size=13))
        with me.box(style=me.Style(
            background=ACCENT_PURPLE,
            padding=me.Padding.symmetric(horizontal=12, vertical=4),
            border_radius=12,
        )):
            me.text(state.work_category.replace("_", " ").title(), style=me.Style(
                color=TEXT_PRIMARY,
                font_size=12,
                font_weight="bold",
            ))

    # Payment provider bids
    me.text("Payment Provider Bids:", style=me.Style(color=TEXT_SECONDARY, margin=me.Margin(bottom=12)))

    for bid in state.payment_provider_bids:
        is_selected = bid.get("provider_id") == state.selected_payment_provider.get("provider_id")
        net_fee = bid.get("net_fee_percent", 0)
        is_cashback = net_fee < 0

        with me.box(style=me.Style(
            background=ACCENT_GREEN + "22" if is_selected else "#1e293b",
            border_radius=8,
            padding=me.Padding.all(16),
            margin=me.Margin(bottom=8),
            border=me.Border.all(me.BorderSide(width=2, color=ACCENT_GREEN if is_selected else "transparent")),
        )):
            with me.box(style=me.Style(display="flex", justify_content="space-between", align_items="center")):
                with me.box():
                    with me.box(style=me.Style(display="flex", align_items="center", gap=8)):
                        me.text(bid.get("provider_name", "Unknown"), style=me.Style(
                            font_size=16,
                            font_weight="bold",
                            color=TEXT_PRIMARY,
                        ))
                        if is_selected:
                            with me.box(style=me.Style(
                                background=ACCENT_GREEN,
                                padding=me.Padding.symmetric(horizontal=8, vertical=2),
                                border_radius=4,
                            )):
                                me.text("BEST RATE", style=me.Style(font_size=9, color=TEXT_PRIMARY, font_weight="bold"))

                    # Fraud protection badge
                    fraud_level = bid.get("fraud_protection", "basic")
                    fraud_colors = {"basic": "#6b7280", "standard": ACCENT_ORANGE, "advanced": ACCENT_GREEN}
                    with me.box(style=me.Style(margin=me.Margin(top=4))):
                        me.text(f"Fraud Protection: {fraud_level.title()}", style=me.Style(
                            font_size=11,
                            color=fraud_colors.get(fraud_level, "#6b7280"),
                        ))

                # Fee breakdown
                with me.box(style=me.Style(text_align="right")):
                    with me.box(style=me.Style(display="flex", gap=16)):
                        with me.box():
                            me.text("Base Fee", style=me.Style(font_size=10, color=TEXT_SECONDARY))
                            me.text(f"{bid.get('base_fee_percent', 0):.1f}%", style=me.Style(
                                font_size=14,
                                color=TEXT_PRIMARY,
                            ))
                        with me.box():
                            me.text("Reward", style=me.Style(font_size=10, color=TEXT_SECONDARY))
                            me.text(f"-{bid.get('reward_percent', 0):.1f}%", style=me.Style(
                                font_size=14,
                                color=ACCENT_GREEN,
                            ))
                        with me.box():
                            me.text("Net Cost", style=me.Style(font_size=10, color=TEXT_SECONDARY))
                            if is_cashback:
                                me.text(f"{net_fee:.1f}%", style=me.Style(
                                    font_size=18,
                                    font_weight="bold",
                                    color=ACCENT_GREEN,
                                ))
                                me.text("CASHBACK!", style=me.Style(
                                    font_size=10,
                                    color=ACCENT_GREEN,
                                    font_weight="bold",
                                ))
                            else:
                                me.text(f"{net_fee:.1f}%", style=me.Style(
                                    font_size=18,
                                    font_weight="bold",
                                    color=ACCENT_ORANGE if net_fee > 1 else TEXT_PRIMARY,
                                ))

    # Summary of selection
    if state.selected_payment_provider:
        with me.box(style=me.Style(
            background="#0f172a",
            border_radius=8,
            padding=me.Padding.all(16),
            margin=me.Margin(top=16),
            border=me.Border.all(me.BorderSide(width=1, color=ACCENT_GREEN)),
        )):
            me.text("Selected Provider:", style=me.Style(color=TEXT_SECONDARY, font_size=11, margin=me.Margin(bottom=8)))
            with me.box(style=me.Style(display="flex", justify_content="space-between", align_items="center")):
                me.text(state.selected_payment_provider.get("provider_name", ""), style=me.Style(
                    font_size=16,
                    font_weight="bold",
                    color=TEXT_PRIMARY,
                ))
                net = state.payment_net_cost
                if net < 0:
                    me.text(f"You EARN {abs(net):.1f}% cashback on ${state.agreed_price:.2f}", style=me.Style(
                        color=ACCENT_GREEN,
                        font_weight="bold",
                    ))
                else:
                    me.text(f"Net fee: {net:.1f}% on ${state.agreed_price:.2f}", style=me.Style(
                        color=TEXT_PRIMARY,
                    ))

    me.button(
        "Proceed to Settlement →",
        on_click=on_settle,
        style=me.Style(
            margin=me.Margin(top=20),
            background="#ec4899",
            color=TEXT_PRIMARY,
            padding=me.Padding.symmetric(horizontal=24, vertical=12),
            border_radius=8,
        ),
    )


def render_step_7():
    """Step 7: Settlement - Transaction Receipt."""
    state = me.state(State)

    me.text("Settlement Complete", style=me.Style(font_size=18, font_weight="bold", color=ACCENT_GREEN, margin=me.Margin(bottom=4)))
    me.text("Transaction Receipt", style=me.Style(font_size=12, color=TEXT_SECONDARY, margin=me.Margin(bottom=16)))

    with me.box(style=me.Style(background="#1e293b", padding=me.Padding.all(20), border_radius=8, margin=me.Margin(bottom=16))):
        # Transaction Header
        with me.box(style=me.Style(
            border=me.Border(bottom=me.BorderSide(width=1, color="#374151")),
            padding=me.Padding(bottom=12),
            margin=me.Margin(bottom=12),
        )):
            me.text("TRANSACTION DETAILS", style=me.Style(font_size=11, color=TEXT_SECONDARY, letter_spacing="1px", margin=me.Margin(bottom=8)))

            with me.box(style=me.Style(display="flex", justify_content="space-between", margin=me.Margin(bottom=4))):
                me.text("Transaction ID:", style=me.Style(color=TEXT_SECONDARY, font_size=13))
                me.text(f"txn-{state.contract_id}", style=me.Style(color=ACCENT_CYAN, font_family="monospace", font_size=13))

            with me.box(style=me.Style(display="flex", justify_content="space-between", margin=me.Margin(bottom=4))):
                me.text("Timestamp:", style=me.Style(color=TEXT_SECONDARY, font_size=13))
                me.text(state.settlement_timestamp, style=me.Style(color=TEXT_PRIMARY, font_size=13))

            with me.box(style=me.Style(display="flex", justify_content="space-between", margin=me.Margin(bottom=4))):
                me.text("Contract ID:", style=me.Style(color=TEXT_SECONDARY, font_size=13))
                me.text(state.contract_id, style=me.Style(color=TEXT_PRIMARY, font_family="monospace", font_size=13))

        # Parties Section
        with me.box(style=me.Style(
            border=me.Border(bottom=me.BorderSide(width=1, color="#374151")),
            padding=me.Padding(bottom=12),
            margin=me.Margin(bottom=12),
        )):
            me.text("PARTIES", style=me.Style(font_size=11, color=TEXT_SECONDARY, letter_spacing="1px", margin=me.Margin(bottom=8)))

            with me.box(style=me.Style(display="flex", justify_content="space-between", margin=me.Margin(bottom=4))):
                me.text("Consumer ID:", style=me.Style(color=TEXT_SECONDARY, font_size=13))
                me.text("consumer-demo-001", style=me.Style(color=ACCENT_PURPLE, font_family="monospace", font_size=13))

            with me.box(style=me.Style(display="flex", justify_content="space-between", margin=me.Margin(bottom=4))):
                me.text("Provider ID:", style=me.Style(color=TEXT_SECONDARY, font_size=13))
                me.text(state.winner_provider_id, style=me.Style(color=ACCENT_ORANGE, font_family="monospace", font_size=13))

            with me.box(style=me.Style(display="flex", justify_content="space-between", margin=me.Margin(bottom=4))):
                me.text("Provider Name:", style=me.Style(color=TEXT_SECONDARY, font_size=13))
                me.text(state.winner_provider_name, style=me.Style(color=TEXT_PRIMARY, font_size=13))

            with me.box(style=me.Style(display="flex", justify_content="space-between")):
                me.text("Provider Tier:", style=me.Style(color=TEXT_SECONDARY, font_size=13))
                me.text(state.winner_tier, style=me.Style(color=ACCENT_CYAN, font_size=13))

        # Execution Section
        with me.box(style=me.Style(
            border=me.Border(bottom=me.BorderSide(width=1, color="#374151")),
            padding=me.Padding(bottom=12),
            margin=me.Margin(bottom=12),
        )):
            me.text("EXECUTION", style=me.Style(font_size=11, color=TEXT_SECONDARY, letter_spacing="1px", margin=me.Margin(bottom=8)))

            with me.box(style=me.Style(display="flex", justify_content="space-between", margin=me.Margin(bottom=4))):
                me.text("Status:", style=me.Style(color=TEXT_SECONDARY, font_size=13))
                me.text("COMPLETED", style=me.Style(color=ACCENT_GREEN, font_weight="bold", font_size=13))

            with me.box(style=me.Style(display="flex", justify_content="space-between")):
                me.text("Execution Time:", style=me.Style(color=TEXT_SECONDARY, font_size=13))
                me.text(f"{state.execution_time_ms:,}ms", style=me.Style(color=TEXT_PRIMARY, font_size=13))

        # Payment Provider Section
        if state.selected_payment_provider:
            with me.box(style=me.Style(
                border=me.Border(bottom=me.BorderSide(width=1, color="#374151")),
                padding=me.Padding(bottom=12),
                margin=me.Margin(bottom=12),
            )):
                with me.box(style=me.Style(display="flex", align_items="center", gap=8, margin=me.Margin(bottom=8))):
                    me.text("PAYMENT PROVIDER", style=me.Style(font_size=11, color=TEXT_SECONDARY, letter_spacing="1px"))
                    is_cashback = state.payment_net_cost < 0
                    with me.box(style=me.Style(
                        background=ACCENT_GREEN if is_cashback else "#f97316",
                        padding=me.Padding.symmetric(horizontal=8, vertical=2),
                        border_radius=4,
                    )):
                        me.text("CASHBACK" if is_cashback else "PROCESSING", style=me.Style(font_size=9, color=TEXT_PRIMARY, font_weight="bold"))

                with me.box(style=me.Style(display="flex", justify_content="space-between", margin=me.Margin(bottom=4))):
                    me.text("Provider:", style=me.Style(color=TEXT_SECONDARY, font_size=13))
                    me.text(state.selected_payment_provider.get("provider_name", ""), style=me.Style(color=TEXT_PRIMARY, font_size=13))

                with me.box(style=me.Style(display="flex", justify_content="space-between", margin=me.Margin(bottom=4))):
                    me.text("Work Category:", style=me.Style(color=TEXT_SECONDARY, font_size=13))
                    me.text(state.work_category.replace("_", " ").title(), style=me.Style(color=ACCENT_PURPLE, font_size=13))

                with me.box(style=me.Style(display="flex", justify_content="space-between", margin=me.Margin(bottom=4))):
                    me.text("Base Fee:", style=me.Style(color=TEXT_SECONDARY, font_size=13))
                    me.text(f"{state.payment_base_fee:.1f}%", style=me.Style(color=TEXT_PRIMARY, font_size=13))

                with me.box(style=me.Style(display="flex", justify_content="space-between", margin=me.Margin(bottom=4))):
                    me.text("Category Reward:", style=me.Style(color=TEXT_SECONDARY, font_size=13))
                    me.text(f"-{state.payment_reward:.1f}%", style=me.Style(color=ACCENT_GREEN, font_size=13))

                with me.box(style=me.Style(display="flex", justify_content="space-between")):
                    me.text("Net Payment Cost:", style=me.Style(color=TEXT_SECONDARY, font_size=13))
                    if state.payment_net_cost < 0:
                        me.text(f"{state.payment_net_cost:.1f}% (CASHBACK!)", style=me.Style(color=ACCENT_GREEN, font_weight="bold", font_size=13))
                    else:
                        me.text(f"{state.payment_net_cost:.1f}%", style=me.Style(color=TEXT_PRIMARY, font_size=13))

        # AP2 Payment Section (if enabled)
        if state.ap2_enabled:
            with me.box(style=me.Style(
                border=me.Border(bottom=me.BorderSide(width=1, color="#374151")),
                padding=me.Padding(bottom=12),
                margin=me.Margin(bottom=12),
            )):
                with me.box(style=me.Style(display="flex", align_items="center", gap=8, margin=me.Margin(bottom=8))):
                    me.text("AP2 PAYMENT", style=me.Style(font_size=11, color=TEXT_SECONDARY, letter_spacing="1px"))
                    with me.box(style=me.Style(
                        background=ACCENT_GREEN,
                        padding=me.Padding.symmetric(horizontal=8, vertical=2),
                        border_radius=4,
                    )):
                        me.text("ENABLED", style=me.Style(font_size=9, color=TEXT_PRIMARY, font_weight="bold"))

                with me.box(style=me.Style(display="flex", justify_content="space-between", margin=me.Margin(bottom=4))):
                    me.text("Payment Method:", style=me.Style(color=TEXT_SECONDARY, font_size=13))
                    me.text(state.payment_method or "card", style=me.Style(color=TEXT_PRIMARY, font_size=13))

                with me.box(style=me.Style(display="flex", justify_content="space-between", margin=me.Margin(bottom=4))):
                    me.text("Mandate ID:", style=me.Style(color=TEXT_SECONDARY, font_size=13))
                    me.text(state.payment_mandate_id, style=me.Style(color=ACCENT_CYAN, font_family="monospace", font_size=11))

                with me.box(style=me.Style(display="flex", justify_content="space-between", margin=me.Margin(bottom=4))):
                    me.text("Receipt ID:", style=me.Style(color=TEXT_SECONDARY, font_size=13))
                    me.text(state.payment_receipt_id, style=me.Style(color=ACCENT_PURPLE, font_family="monospace", font_size=11))

                with me.box(style=me.Style(display="flex", justify_content="space-between")):
                    me.text("Transaction ID:", style=me.Style(color=TEXT_SECONDARY, font_size=13))
                    me.text(state.payment_transaction_id, style=me.Style(color=ACCENT_ORANGE, font_family="monospace", font_size=11))

        # Financial Section
        me.text("FINANCIAL SETTLEMENT", style=me.Style(font_size=11, color=TEXT_SECONDARY, letter_spacing="1px", margin=me.Margin(bottom=8)))

        with me.box(style=me.Style(display="flex", justify_content="space-between", margin=me.Margin(bottom=4))):
            me.text("Agreed Price:", style=me.Style(color=TEXT_SECONDARY, font_size=13))
            me.text(f"${state.agreed_price:.2f}", style=me.Style(color=TEXT_PRIMARY, font_weight="bold", font_size=13))

        with me.box(style=me.Style(display="flex", justify_content="space-between", margin=me.Margin(bottom=4))):
            me.text("Platform Fee (15%):", style=me.Style(color=TEXT_SECONDARY, font_size=13))
            me.text(f"-${state.platform_fee:.2f}", style=me.Style(color="#ef4444", font_size=13))

        with me.box(style=me.Style(
            display="flex",
            justify_content="space-between",
            padding=me.Padding(top=8),
            border=me.Border(top=me.BorderSide(width=1, color="#374151")),
        )):
            me.text("Provider Payout:", style=me.Style(color=TEXT_PRIMARY, font_weight="bold", font_size=14))
            me.text(f"${state.provider_payout:.2f}", style=me.Style(color=ACCENT_GREEN, font_weight="bold", font_size=16))

    me.button(
        "New Request",
        on_click=on_restart,
        style=me.Style(background=ACCENT_BLUE, color=TEXT_PRIMARY, padding=me.Padding.symmetric(horizontal=24, vertical=12), border_radius=8),
    )


@me.page(path="/", on_load=on_page_load)
def home():
    """Main dashboard page."""
    state = me.state(State)

    with me.box(style=me.Style(
        min_height="100vh",
        background=f"linear-gradient(180deg, {DARK_BG} 0%, #0f172a 100%)",
        padding=me.Padding.all(24),
    )):
        # Header
        with me.box(style=me.Style(
            display="flex",
            justify_content="space-between",
            align_items="center",
            margin=me.Margin(bottom=24),
        )):
            with me.box():
                me.text("Agent Exchange", style=me.Style(
                    font_size=28,
                    font_weight="bold",
                    color=TEXT_PRIMARY,
                ))
                me.text("Programmatic Marketplace for AI Agents", style=me.Style(
                    font_size=14,
                    color=TEXT_SECONDARY,
                ))

            with me.box(style=me.Style(
                background=ACCENT_GREEN,
                padding=me.Padding.symmetric(horizontal=16, vertical=8),
                border_radius=20,
            )):
                me.text("LIVE", style=me.Style(color=TEXT_PRIMARY, font_weight="bold", font_size=12))

        # KPI Cards
        with me.box(style=me.Style(
            display="flex",
            gap=16,
            margin=me.Margin(bottom=24),
            flex_wrap="wrap",
        )):
            render_kpi_card(f"{state.total_transactions}" if state.total_transactions > 0 else "-", "Total Transactions", ACCENT_CYAN)
            render_kpi_card(f"${state.platform_revenue:.0f}" if state.platform_revenue > 0 else "-", "Platform Revenue", ACCENT_GREEN)
            render_kpi_card(f"{state.avg_response_time}ms" if state.avg_response_time > 0 else "-", "Avg Response Time", ACCENT_ORANGE)
            success_rate = (state.successful_transactions / state.total_transactions * 100) if state.total_transactions > 0 else 0
            render_kpi_card(f"{success_rate:.1f}%" if state.total_transactions > 0 else "-", "Success Rate", ACCENT_PURPLE)

        # Workflow Topology
        render_topology()

        # Main content area
        with me.box(style=me.Style(
            display="flex",
            gap=24,
            flex_wrap="wrap",
        )):
            # Left: Step content
            with me.box(style=me.Style(flex_grow=1, min_width=400)):
                render_step_content()

            # Right: Provider cards
            with me.box(style=me.Style(width=280)):
                with me.box(style=me.Style(display="flex", justify_content="space-between", align_items="center", margin=me.Margin(bottom=12))):
                    me.text("Registered Providers", style=me.Style(
                        font_size=14,
                        font_weight="bold",
                        color=TEXT_PRIMARY,
                    ))
                    me.button(
                        "Refresh",
                        on_click=on_refresh_providers,
                        style=me.Style(
                            background="transparent",
                            color=ACCENT_CYAN,
                            font_size=11,
                            padding=me.Padding.symmetric(horizontal=8, vertical=4),
                            border=me.Border.all(me.BorderSide(width=1, color=ACCENT_CYAN)),
                            border_radius=4,
                        ),
                    )

                # Show provider count from registry
                provider_count = len(state.registered_providers)
                if provider_count > 0:
                    me.text(f"{provider_count} agents from AEX Registry", style=me.Style(
                        font_size=11,
                        color=ACCENT_GREEN,
                        margin=me.Margin(bottom=8),
                    ))

                with me.box(style=me.Style(display="flex", flex_direction="column", gap=12)):
                    # Use dynamic providers from state - grouped by type
                    if state.registered_providers:
                        # Separate providers by type
                        legal_providers = [p for p in state.registered_providers if p.get("provider_type") != "payment"]
                        payment_providers = [p for p in state.registered_providers if p.get("provider_type") == "payment"]

                        # Legal Work Agents Group
                        if legal_providers:
                            with me.box(style=me.Style(
                                background=CARD_BG,
                                border_radius=8,
                                border=me.Border.all(me.BorderSide(width=1, color=CARD_BORDER)),
                                overflow="hidden",
                            )):
                                # Group header - clickable
                                with me.box(
                                    style=me.Style(
                                        display="flex",
                                        justify_content="space-between",
                                        align_items="center",
                                        padding=me.Padding.all(12),
                                        background="#1e3a5f",
                                        cursor="pointer",
                                    ),
                                    on_click=on_toggle_legal_providers,
                                ):
                                    with me.box(style=me.Style(display="flex", align_items="center", gap=8)):
                                        me.icon("gavel", style=me.Style(color=ACCENT_BLUE, font_size=18))
                                        me.text(f"Legal Work Agents ({len(legal_providers)})", style=me.Style(
                                            font_weight="bold",
                                            color=TEXT_PRIMARY,
                                            font_size=13,
                                        ))
                                    me.icon(
                                        "expand_more" if state.legal_providers_expanded else "chevron_right",
                                        style=me.Style(color=TEXT_SECONDARY, font_size=18)
                                    )

                                # Group content - expandable
                                if state.legal_providers_expanded:
                                    with me.box(style=me.Style(padding=me.Padding.all(12), display="flex", flex_direction="column", gap=8)):
                                        for provider in legal_providers:
                                            render_provider_card_from_dict(provider)

                        # Payment Providers Group
                        if payment_providers:
                            with me.box(style=me.Style(
                                background=CARD_BG,
                                border_radius=8,
                                border=me.Border.all(me.BorderSide(width=1, color=CARD_BORDER)),
                                overflow="hidden",
                            )):
                                # Group header - clickable
                                with me.box(
                                    style=me.Style(
                                        display="flex",
                                        justify_content="space-between",
                                        align_items="center",
                                        padding=me.Padding.all(12),
                                        background="#1e5f3a",
                                        cursor="pointer",
                                    ),
                                    on_click=on_toggle_payment_providers,
                                ):
                                    with me.box(style=me.Style(display="flex", align_items="center", gap=8)):
                                        me.icon("payments", style=me.Style(color=ACCENT_GREEN, font_size=18))
                                        me.text(f"Payment Providers ({len(payment_providers)})", style=me.Style(
                                            font_weight="bold",
                                            color=TEXT_PRIMARY,
                                            font_size=13,
                                        ))
                                    me.icon(
                                        "expand_more" if state.payment_providers_expanded else "chevron_right",
                                        style=me.Style(color=TEXT_SECONDARY, font_size=18)
                                    )

                                # Group content - expandable
                                if state.payment_providers_expanded:
                                    with me.box(style=me.Style(padding=me.Padding.all(12), display="flex", flex_direction="column", gap=8)):
                                        for provider in payment_providers:
                                            render_provider_card_from_dict(provider)
                    else:
                        # No providers registered yet
                        with me.box(style=me.Style(
                            background=CARD_BG,
                            border_radius=8,
                            padding=me.Padding.all(16),
                            border=me.Border.all(me.BorderSide(width=1, color=CARD_BORDER)),
                            text_align="center",
                        )):
                            me.text("No providers registered", style=me.Style(color=TEXT_SECONDARY, font_size=13, margin=me.Margin(bottom=8)))
                            me.text("Start agents and they will appear here", style=me.Style(color=TEXT_SECONDARY, font_size=11))


# ============================================================================
# LANDING PAGE - Animated Flow Visualization
# ============================================================================

@me.stateclass
class LandingState:
    """Landing page state."""
    active_step: int = 0
    auto_play: bool = True


def on_step_click(e: me.ClickEvent):
    """Handle step click."""
    state = me.state(LandingState)
    state.active_step = int(e.key)
    state.auto_play = False


def on_next_step(e: me.ClickEvent):
    """Advance to next step."""
    state = me.state(LandingState)
    state.active_step = (state.active_step + 1) % 6
    state.auto_play = False


def on_auto_play(e: me.ClickEvent):
    """Toggle auto-play."""
    state = me.state(LandingState)
    state.auto_play = not state.auto_play
    if state.auto_play:
        state.active_step = (state.active_step + 1) % 6


STEP_DATA = [
    {
        "num": 1,
        "title": "PUBLISH WORK",
        "desc": "Consumer Agent sends a task request with requirements to AEX",
        "color": ACCENT_BLUE,
        "icon": "description",
    },
    {
        "num": 2,
        "title": "BROADCAST",
        "desc": "AEX broadcasts the opportunity to all matching Provider Agents",
        "color": ACCENT_ORANGE,
        "icon": "cell_tower",
    },
    {
        "num": 3,
        "title": "SUBMIT BIDS",
        "desc": "Providers submit competitive bids with price and confidence scores",
        "color": ACCENT_PURPLE,
        "icon": "gavel",
    },
    {
        "num": 4,
        "title": "EVALUATE & AWARD",
        "desc": "AEX scores bids on price, trust, and quality - awards contract to winner",
        "color": ACCENT_CYAN,
        "icon": "emoji_events",
    },
    {
        "num": 5,
        "title": "DIRECT EXECUTION",
        "desc": "AEX steps aside - Consumer and Provider communicate directly via A2A",
        "color": ACCENT_GREEN,
        "icon": "sync_alt",
    },
    {
        "num": 6,
        "title": "SETTLEMENT",
        "desc": "Upon completion, AEX processes payment with 15% platform fee",
        "color": "#ec4899",
        "icon": "account_balance_wallet",
    },
]


def render_flow_step(step_num: int, active: bool, step_data: dict):
    """Render a single flow step indicator."""
    is_active = active
    with me.box(
        key=str(step_num - 1),
        on_click=on_step_click,
        style=me.Style(
            display="flex",
            flex_direction="column",
            align_items="center",
            cursor="pointer",
            opacity=1.0 if is_active else 0.4,
            transition="all 0.3s ease",
        ),
    ):
        # Step circle
        with me.box(style=me.Style(
            width=60,
            height=60,
            border_radius="50%",
            background=step_data["color"] if is_active else "#1e293b",
            border=me.Border.all(me.BorderSide(width=3, color=step_data["color"])),
            display="flex",
            align_items="center",
            justify_content="center",
            box_shadow=f"0 0 20px {step_data['color']}66" if is_active else "none",
            transition="all 0.3s ease",
        )):
            me.icon(step_data["icon"], style=me.Style(color=TEXT_PRIMARY if is_active else step_data["color"], font_size=28))

        me.text(f"{step_num}", style=me.Style(
            font_size=12,
            font_weight="bold",
            color=step_data["color"],
            margin=me.Margin(top=8),
        ))
        me.text(step_data["title"], style=me.Style(
            font_size=10,
            color=TEXT_PRIMARY if is_active else TEXT_SECONDARY,
            text_align="center",
            max_width=80,
        ))


def render_agent_node(name: str, icon: str, color: str, is_active: bool, subtitle: str = ""):
    """Render an agent node (Consumer or Provider)."""
    with me.box(style=me.Style(
        display="flex",
        flex_direction="column",
        align_items="center",
        padding=me.Padding.all(16),
        opacity=1.0 if is_active else 0.5,
        transition="all 0.3s ease",
    )):
        with me.box(style=me.Style(
            width=80,
            height=80,
            border_radius="50%",
            background=f"linear-gradient(135deg, {color}44, {color}22)",
            border=me.Border.all(me.BorderSide(width=2, color=color)),
            display="flex",
            align_items="center",
            justify_content="center",
            box_shadow=f"0 0 30px {color}44" if is_active else "none",
            transition="all 0.3s ease",
        )):
            me.icon(icon, style=me.Style(color=color, font_size=40))

        me.text(name, style=me.Style(
            font_size=14,
            font_weight="bold",
            color=TEXT_PRIMARY,
            margin=me.Margin(top=12),
            text_align="center",
        ))
        if subtitle:
            me.text(subtitle, style=me.Style(
                font_size=11,
                color=TEXT_SECONDARY,
                text_align="center",
            ))


def render_aex_hub(is_active: bool, active_step: int):
    """Render the central AEX hub."""
    # Determine hub color based on step
    hub_color = STEP_DATA[active_step]["color"] if is_active else ACCENT_ORANGE

    with me.box(style=me.Style(
        display="flex",
        flex_direction="column",
        align_items="center",
    )):
        # Outer glow ring
        with me.box(style=me.Style(
            width=140,
            height=140,
            border_radius="50%",
            background=f"radial-gradient(circle, {hub_color}33 0%, transparent 70%)",
            display="flex",
            align_items="center",
            justify_content="center",
        )):
            # Main hub circle
            with me.box(style=me.Style(
                width=100,
                height=100,
                border_radius="50%",
                background=f"linear-gradient(135deg, {hub_color}, {hub_color}aa)",
                display="flex",
                align_items="center",
                justify_content="center",
                box_shadow=f"0 0 40px {hub_color}66",
            )):
                me.text("AEX", style=me.Style(
                    font_size=28,
                    font_weight="bold",
                    color=TEXT_PRIMARY,
                ))

        me.text("Agent Exchange", style=me.Style(
            font_size=12,
            color=TEXT_SECONDARY,
            margin=me.Margin(top=12),
        ))


def render_bid_card(provider: str, price: str, confidence: str, color: str, show: bool):
    """Render a floating bid card."""
    with me.box(style=me.Style(
        background=CARD_BG,
        border=me.Border.all(me.BorderSide(width=1, color=color)),
        border_radius=8,
        padding=me.Padding.all(10),
        margin=me.Margin(bottom=8),
        opacity=1.0 if show else 0.0,
        transition="all 0.5s ease",
    )):
        me.text(provider, style=me.Style(font_size=12, font_weight="bold", color=TEXT_PRIMARY))
        with me.box(style=me.Style(display="flex", gap=12)):
            me.text(f"Bid: {price}", style=me.Style(font_size=11, color=ACCENT_GREEN))
            me.text(f"Conf: {confidence}", style=me.Style(font_size=11, color=ACCENT_CYAN))


def render_connection_line(from_side: str, to_side: str, is_active: bool, color: str, label: str = ""):
    """Render a connection line between nodes."""
    with me.box(style=me.Style(
        flex_grow=1,
        display="flex",
        flex_direction="column",
        align_items="center",
    )):
        with me.box(style=me.Style(
            width="100%",
            height=4,
            background=f"linear-gradient(90deg, {color}00, {color}, {color}00)" if is_active else "#1e293b",
            border_radius=2,
            transition="all 0.5s ease",
            box_shadow=f"0 0 10px {color}" if is_active else "none",
        )):
            pass
        if label:
            me.text(label, style=me.Style(
                font_size=10,
                color=color if is_active else TEXT_SECONDARY,
                margin=me.Margin(top=4),
            ))


@me.page(path="/landing")
def landing():
    """Landing page with animated flow visualization."""
    state = me.state(LandingState)

    # CSS for animations
    me.html("""
    <style>
        @keyframes pulse {
            0%, 100% { transform: scale(1); opacity: 1; }
            50% { transform: scale(1.05); opacity: 0.8; }
        }
        @keyframes flowRight {
            0% { background-position: -200% 0; }
            100% { background-position: 200% 0; }
        }
        @keyframes flowLeft {
            0% { background-position: 200% 0; }
            100% { background-position: -200% 0; }
        }
        .flow-line-active {
            background: linear-gradient(90deg, transparent, #10b981, transparent);
            background-size: 200% 100%;
            animation: flowRight 1.5s infinite;
        }
    </style>
    """)

    with me.box(style=me.Style(
        min_height="100vh",
        background=f"linear-gradient(180deg, {DARK_BG} 0%, #0f172a 50%, {DARK_BG} 100%)",
        padding=me.Padding.all(24),
    )):
        # Header
        with me.box(style=me.Style(text_align="center", margin=me.Margin(bottom=32))):
            me.text("Agent Exchange", style=me.Style(
                font_size=42,
                font_weight="bold",
                color=TEXT_PRIMARY,
                margin=me.Margin(bottom=8),
            ))
            me.text("The Programmatic Marketplace for AI Agents", style=me.Style(
                font_size=18,
                color=TEXT_SECONDARY,
                margin=me.Margin(bottom=4),
            ))
            me.text("Ad-tech economics applied to agentic AI", style=me.Style(
                font_size=14,
                color=ACCENT_CYAN,
            ))

        # Main visualization container
        with me.box(style=me.Style(
            background=CARD_BG,
            border_radius=16,
            padding=me.Padding.all(32),
            max_width=1100,
            margin=me.Margin(left="auto", right="auto", bottom=32),
            border=me.Border.all(me.BorderSide(width=1, color="#1e293b")),
        )):
            # Flow visualization - 3 column layout
            with me.box(style=me.Style(
                display="flex",
                align_items="center",
                justify_content="space-between",
                margin=me.Margin(bottom=40),
                min_height=250,
            )):
                # Left: Consumer Agent
                with me.box(style=me.Style(width=150, display="flex", flex_direction="column", align_items="center")):
                    render_agent_node(
                        "Consumer Agent",
                        "smart_toy",
                        ACCENT_BLUE,
                        state.active_step in [0, 4, 5],
                        "Task Requester"
                    )
                    # Request snippet for step 1
                    if state.active_step == 0:
                        with me.box(style=me.Style(
                            background="#0f172a",
                            border_radius=8,
                            padding=me.Padding.all(10),
                            margin=me.Margin(top=12),
                            border=me.Border.all(me.BorderSide(width=1, color=ACCENT_BLUE)),
                        )):
                            me.text("Request:", style=me.Style(font_size=10, color=TEXT_SECONDARY))
                            me.text('{"category": "legal",', style=me.Style(font_size=9, color=ACCENT_GREEN, font_family="monospace"))
                            me.text(' "budget": 50.00}', style=me.Style(font_size=9, color=ACCENT_GREEN, font_family="monospace"))

                # Connection: Consumer → AEX
                with me.box(style=me.Style(flex_grow=1, display="flex", flex_direction="column", align_items="center")):
                    with me.box(style=me.Style(
                        width="100%",
                        height=4,
                        background=f"linear-gradient(90deg, {ACCENT_BLUE}00, {ACCENT_BLUE}, {ACCENT_ORANGE})" if state.active_step == 0 else (
                            f"linear-gradient(270deg, {ACCENT_GREEN}00, {ACCENT_GREEN}, {ACCENT_BLUE})" if state.active_step == 4 else "#1e293b"
                        ),
                        border_radius=2,
                        box_shadow=f"0 0 10px {ACCENT_BLUE}" if state.active_step in [0, 4] else "none",
                        transition="all 0.5s ease",
                    )):
                        pass
                    if state.active_step == 0:
                        me.text("1. Publish", style=me.Style(font_size=10, color=ACCENT_BLUE, margin=me.Margin(top=4)))
                    elif state.active_step == 4:
                        me.text("5. A2A Direct", style=me.Style(font_size=10, color=ACCENT_GREEN, margin=me.Margin(top=4)))

                # Center: AEX Hub
                with me.box(style=me.Style(width=180)):
                    render_aex_hub(True, state.active_step)

                    # Show winner badge for step 3
                    if state.active_step == 3:
                        with me.box(style=me.Style(
                            display="flex",
                            justify_content="center",
                            margin=me.Margin(top=12),
                        )):
                            with me.box(style=me.Style(
                                background=ACCENT_CYAN,
                                border_radius=16,
                                padding=me.Padding.symmetric(horizontal=12, vertical=4),
                            )):
                                me.text("WINNER SELECTED", style=me.Style(font_size=10, font_weight="bold", color=TEXT_PRIMARY))

                # Connection: AEX → Providers
                with me.box(style=me.Style(flex_grow=1, display="flex", flex_direction="column", align_items="center")):
                    with me.box(style=me.Style(
                        width="100%",
                        height=4,
                        background=f"linear-gradient(90deg, {ACCENT_ORANGE}, {ACCENT_ORANGE}, {ACCENT_PURPLE}00)" if state.active_step == 1 else (
                            f"linear-gradient(270deg, {ACCENT_PURPLE}, {ACCENT_PURPLE}, {ACCENT_CYAN}00)" if state.active_step == 2 else "#1e293b"
                        ),
                        border_radius=2,
                        box_shadow=f"0 0 10px {ACCENT_ORANGE}" if state.active_step in [1, 2] else "none",
                        transition="all 0.5s ease",
                    )):
                        pass
                    if state.active_step == 1:
                        me.text("2. Broadcast", style=me.Style(font_size=10, color=ACCENT_ORANGE, margin=me.Margin(top=4)))
                    elif state.active_step == 2:
                        me.text("3. Submit Bids", style=me.Style(font_size=10, color=ACCENT_PURPLE, margin=me.Margin(top=4)))

                # Right: Provider Agents
                with me.box(style=me.Style(width=200, display="flex", flex_direction="column", align_items="center")):
                    me.text("Provider Agents", style=me.Style(
                        font_size=14,
                        font_weight="bold",
                        color=TEXT_PRIMARY,
                        margin=me.Margin(bottom=12),
                    ))

                    # Provider icons row
                    with me.box(style=me.Style(display="flex", gap=8, margin=me.Margin(bottom=12))):
                        for icon, color in [("gavel", ACCENT_PURPLE), ("account_balance", ACCENT_ORANGE), ("business", ACCENT_CYAN)]:
                            with me.box(style=me.Style(
                                width=45,
                                height=45,
                                border_radius="50%",
                                background=f"{color}22",
                                border=me.Border.all(me.BorderSide(width=1, color=color)),
                                display="flex",
                                align_items="center",
                                justify_content="center",
                                opacity=1.0 if state.active_step in [1, 2, 4] else 0.4,
                                transition="all 0.3s ease",
                            )):
                                me.icon(icon, style=me.Style(color=color, font_size=22))

                    # Bid cards (show during bid step)
                    if state.active_step == 2:
                        render_bid_card("Legal Agent A", "$15.00", "0.85", ACCENT_PURPLE, True)
                        render_bid_card("Legal Agent B", "$22.50", "0.92", ACCENT_CYAN, True)
                        render_bid_card("Legal Agent C", "$35.00", "0.95", ACCENT_GREEN, True)

            # Step indicators
            with me.box(style=me.Style(
                display="flex",
                justify_content="space-between",
                align_items="flex-start",
                padding=me.Padding.symmetric(horizontal=20),
                margin=me.Margin(bottom=32),
            )):
                for i, step in enumerate(STEP_DATA):
                    render_flow_step(step["num"], state.active_step == i, step)

            # Step description
            with me.box(style=me.Style(
                background="#0f172a",
                border_radius=12,
                padding=me.Padding.all(20),
                border=me.Border.all(me.BorderSide(width=2, color=STEP_DATA[state.active_step]["color"])),
                text_align="center",
            )):
                me.text(f"Step {state.active_step + 1}: {STEP_DATA[state.active_step]['title']}", style=me.Style(
                    font_size=20,
                    font_weight="bold",
                    color=STEP_DATA[state.active_step]["color"],
                    margin=me.Margin(bottom=8),
                ))
                me.text(STEP_DATA[state.active_step]["desc"], style=me.Style(
                    font_size=16,
                    color=TEXT_PRIMARY,
                ))

            # Controls
            with me.box(style=me.Style(
                display="flex",
                justify_content="center",
                gap=16,
                margin=me.Margin(top=24),
            )):
                me.button(
                    "Next Step",
                    on_click=on_next_step,
                    style=me.Style(
                        background=ACCENT_CYAN,
                        color=TEXT_PRIMARY,
                        padding=me.Padding.symmetric(horizontal=32, vertical=12),
                        border_radius=8,
                        font_size=14,
                        font_weight="bold",
                    ),
                )

        # Try Demo CTA
        with me.box(style=me.Style(text_align="center")):
            me.link(
                text="Try the Interactive Demo →",
                url="/",
                style=me.Style(
                    font_size=18,
                    color=ACCENT_GREEN,
                    text_decoration="none",
                ),
            )
            me.text("Experience the full 6-step marketplace flow", style=me.Style(
                font_size=14,
                color=TEXT_SECONDARY,
                margin=me.Margin(top=8),
            ))

        # Value props - highlight based on active step
        # Map: steps 0,1,2 → Bidding, step 3 → Trust, step 4 → A2A, step 5 → Settlement
        value_props = [
            ("speed", "Real-time Bidding", "Sub-second bid collection and evaluation", [0, 1, 2], ACCENT_ORANGE),
            ("security", "Trust Scoring", "Reputation system with verified tiers", [3], ACCENT_CYAN),
            ("sync_alt", "A2A Protocol", "Direct agent-to-agent execution", [4], ACCENT_GREEN),
            ("account_balance_wallet", "Fair Settlement", "Automated payment with 15% platform fee", [5], "#ec4899"),
        ]

        with me.box(style=me.Style(
            display="flex",
            justify_content="center",
            gap=32,
            margin=me.Margin(top=48),
            flex_wrap="wrap",
        )):
            for icon, title, desc, active_steps, color in value_props:
                is_active = state.active_step in active_steps
                with me.box(style=me.Style(
                    background=f"{color}22" if is_active else CARD_BG,
                    border_radius=12,
                    padding=me.Padding.all(20),
                    width=220,
                    text_align="center",
                    border=me.Border.all(me.BorderSide(width=2, color=color if is_active else "#1e293b")),
                    box_shadow=f"0 0 20px {color}44" if is_active else "none",
                    transition="all 0.3s ease",
                    opacity=1.0 if is_active else 0.5,
                )):
                    me.icon(icon, style=me.Style(color=color if is_active else TEXT_SECONDARY, font_size=32))
                    me.text(title, style=me.Style(
                        font_size=16,
                        font_weight="bold",
                        color=TEXT_PRIMARY if is_active else TEXT_SECONDARY,
                        margin=me.Margin(top=12, bottom=8),
                    ))
                    me.text(desc, style=me.Style(
                        font_size=13,
                        color=TEXT_PRIMARY if is_active else TEXT_SECONDARY,
                    ))


# ==================== SIMPLIFIED UI ====================

def on_clear_history(e: me.ClickEvent):
    """Clear task history."""
    state = me.state(State)
    state.task_results = []
    state.log_messages = []


def render_task_result_card(task: TaskResult):
    """Render a single task result card with status, progress, and AP2 payment info."""
    tier_colors = {"VERIFIED": ACCENT_ORANGE, "TRUSTED": ACCENT_BLUE, "PREFERRED": ACCENT_GREEN, "UNVERIFIED": "#666"}

    # Status colors and labels
    status_config = {
        "pending": {"color": "#666", "label": "Pending", "icon": "..."},
        "bidding": {"color": ACCENT_CYAN, "label": "Collecting Bids", "icon": "..."},
        "evaluating": {"color": ACCENT_ORANGE, "label": "Evaluating", "icon": "..."},
        "awarded": {"color": ACCENT_BLUE, "label": "Contract Awarded", "icon": "..."},
        "executing": {"color": ACCENT_GREEN, "label": "Executing", "icon": "..."},
        "paying": {"color": "#a855f7", "label": "Processing Payment", "icon": "$"},
        "settling": {"color": "#f97316", "label": "Settling", "icon": "..."},
        "completed": {"color": ACCENT_GREEN, "label": "Completed", "icon": "OK"},
        "failed": {"color": "#ef4444", "label": "Failed", "icon": "X"},
    }

    status_info = status_config.get(task.status, status_config["pending"])
    is_in_progress = task.status not in ["completed", "failed"]

    with me.box(style=me.Style(
        background=CARD_BG,
        border_radius=12,
        padding=me.Padding.all(16),
        margin=me.Margin(bottom=16),
        border=me.Border.all(me.BorderSide(width=2 if is_in_progress else 1, color=status_info["color"] if is_in_progress else CARD_BORDER)),
    )):
        # Header with task ID, status badge, and timestamp
        with me.box(style=me.Style(display="flex", justify_content="space-between", align_items="center", margin=me.Margin(bottom=12))):
            with me.box(style=me.Style(display="flex", gap=12, align_items="center")):
                me.text(f"Task: {task.task_id}", style=me.Style(font_weight="bold", color=TEXT_PRIMARY))
                # Status badge
                with me.box(style=me.Style(
                    background=status_info["color"],
                    padding=me.Padding.symmetric(horizontal=8, vertical=4),
                    border_radius=12,
                    display="flex",
                    gap=6,
                    align_items="center",
                )):
                    me.text(status_info["icon"], style=me.Style(font_size=10, color=TEXT_PRIMARY))
                    me.text(status_info["label"], style=me.Style(font_size=11, color=TEXT_PRIMARY, font_weight="bold"))
            me.text(task.timestamp if task.timestamp else "In progress...", style=me.Style(font_size=12, color=TEXT_SECONDARY))

        # Progress bar (7 steps)
        if is_in_progress or task.current_step > 0:
            step_labels = ["Bids", "Eval", "Award", "Execute", "AP2 Select", "AP2 Pay", "Settle"]
            with me.box(style=me.Style(margin=me.Margin(bottom=16))):
                # Progress track
                with me.box(style=me.Style(display="flex", gap=4, margin=me.Margin(bottom=4))):
                    for i in range(7):
                        step_completed = i < task.current_step
                        step_active = i == task.current_step - 1
                        with me.box(style=me.Style(
                            flex_grow=1,
                            height=4,
                            background=ACCENT_GREEN if step_completed else (status_info["color"] if step_active else "#334155"),
                            border_radius=2,
                        )):
                            pass
                # Step labels
                with me.box(style=me.Style(display="flex", justify_content="space-between")):
                    for i, label in enumerate(step_labels):
                        step_completed = i < task.current_step
                        me.text(label, style=me.Style(font_size=9, color=TEXT_PRIMARY if step_completed else TEXT_SECONDARY))

        # Task description
        me.text(f'"{task.description[:100]}..."' if len(task.description) > 100 else f'"{task.description}"',
                style=me.Style(color=TEXT_SECONDARY, font_style="italic", margin=me.Margin(bottom=12)))

        # Bids section (only show if we have bids)
        if task.bids:
            me.text("Bids Received:", style=me.Style(font_weight="bold", color=TEXT_PRIMARY, margin=me.Margin(bottom=8)))
            for bid in task.bids:
                is_winner = bid.get("provider_name") == task.winner_name
                with me.box(style=me.Style(
                    display="flex",
                    justify_content="space-between",
                    padding=me.Padding.symmetric(horizontal=8, vertical=4),
                    background="#1e293b" if is_winner else "transparent",
                    border_radius=4,
                    margin=me.Margin(bottom=4),
                )):
                    with me.box(style=me.Style(display="flex", gap=8, align_items="center")):
                        if is_winner:
                            me.text("*", style=me.Style(color=ACCENT_GREEN, font_weight="bold"))
                        me.text(bid.get("provider_name", "Unknown"), style=me.Style(color=TEXT_PRIMARY if is_winner else TEXT_SECONDARY))
                        tier = bid.get("tier", "UNVERIFIED")
                        with me.box(style=me.Style(
                            background=tier_colors.get(tier, "#666"),
                            padding=me.Padding.symmetric(horizontal=4, vertical=2),
                            border_radius=4,
                        )):
                            me.text(tier, style=me.Style(font_size=9, color=TEXT_PRIMARY))
                    with me.box(style=me.Style(display="flex", gap=16)):
                        me.text(f"${bid.get('price', 0):.2f}", style=me.Style(color=TEXT_PRIMARY if is_winner else TEXT_SECONDARY))
                        score = bid.get("score", 0)
                        if score > 0:
                            me.text(f"score: {score:.3f}", style=me.Style(color=TEXT_SECONDARY, font_size=12))

        # Winner and execution info (only show if winner selected)
        if task.winner_name:
            with me.box(style=me.Style(
                background="#1e293b",
                border_radius=8,
                padding=me.Padding.all(12),
                margin=me.Margin(top=12),
            )):
                with me.box(style=me.Style(display="flex", justify_content="space-between", margin=me.Margin(bottom=8))):
                    me.text(f"Winner: {task.winner_name}", style=me.Style(color=ACCENT_GREEN, font_weight="bold"))
                    me.text(f"${task.winner_price:.2f}", style=me.Style(color=ACCENT_GREEN, font_size=18, font_weight="bold"))

                with me.box(style=me.Style(display="flex", gap=24, flex_wrap="wrap")):
                    if task.contract_id:
                        contract_display = task.contract_id[:20] + "..." if len(task.contract_id) > 20 else task.contract_id
                        me.text(f"Contract: {contract_display}", style=me.Style(color=TEXT_SECONDARY, font_size=12))
                    if task.execution_time_ms > 0:
                        me.text(f"Execution: {task.execution_time_ms}ms", style=me.Style(color=TEXT_SECONDARY, font_size=12))
                    if task.platform_fee > 0:
                        me.text(f"Platform fee: ${task.platform_fee:.2f}", style=me.Style(color=TEXT_SECONDARY, font_size=12))

        # AP2 Payment Info (only show if payment was processed)
        if task.ap2_payment_provider:
            with me.box(style=me.Style(
                background="linear-gradient(135deg, #4c1d95 0%, #1e1b4b 100%)",
                border_radius=8,
                padding=me.Padding.all(12),
                margin=me.Margin(top=12),
                border=me.Border.all(me.BorderSide(width=1, color="#7c3aed")),
            )):
                with me.box(style=me.Style(display="flex", justify_content="space-between", align_items="center", margin=me.Margin(bottom=8))):
                    me.text("AP2 Payment", style=me.Style(color="#a78bfa", font_weight="bold", font_size=14))
                    with me.box(style=me.Style(
                        background="#22c55e",
                        padding=me.Padding.symmetric(horizontal=8, vertical=2),
                        border_radius=8,
                    )):
                        me.text("PAID", style=me.Style(font_size=10, color=TEXT_PRIMARY, font_weight="bold"))

                with me.box(style=me.Style(display="grid", gap=8)):
                    with me.box(style=me.Style(display="flex", gap=16, flex_wrap="wrap")):
                        me.text(f"Provider: {task.ap2_payment_provider}", style=me.Style(color=TEXT_PRIMARY, font_size=12))
                        if task.ap2_payment_method:
                            me.text(f"Method: {task.ap2_payment_method}", style=me.Style(color=TEXT_SECONDARY, font_size=12))
                    if task.ap2_cart_mandate_id:
                        me.text(f"Cart Mandate: {task.ap2_cart_mandate_id}", style=me.Style(color=TEXT_SECONDARY, font_size=11))
                    if task.ap2_payment_receipt_id:
                        me.text(f"Receipt: {task.ap2_payment_receipt_id}", style=me.Style(color=TEXT_SECONDARY, font_size=11))

        # Agent response (with markdown-style rendering)
        if task.agent_response:
            me.text("Agent Response:", style=me.Style(font_weight="bold", color=TEXT_PRIMARY, margin=me.Margin(top=12, bottom=4)))

            # Parse and render markdown-style content
            with me.box(style=me.Style(
                background="#0f172a",
                padding=me.Padding.all(12),
                border_radius=8,
                max_height=300,
                overflow_y="auto",
            )):
                # Split response into lines and render with basic formatting
                lines = task.agent_response.split("\n")
                for line in lines[:50]:  # Limit to 50 lines to prevent huge renders
                    line = line.strip()
                    if not line:
                        me.box(style=me.Style(height=8))  # Empty line spacing
                        continue

                    # Detect headers (## or ###)
                    if line.startswith("### "):
                        me.text(line[4:], style=me.Style(
                            color=TEXT_PRIMARY, font_weight="bold", font_size=14,
                            margin=me.Margin(top=8, bottom=4),
                        ))
                    elif line.startswith("## "):
                        me.text(line[3:], style=me.Style(
                            color=TEXT_PRIMARY, font_weight="bold", font_size=16,
                            margin=me.Margin(top=12, bottom=6),
                        ))
                    elif line.startswith("# "):
                        me.text(line[2:], style=me.Style(
                            color=TEXT_PRIMARY, font_weight="bold", font_size=18,
                            margin=me.Margin(top=12, bottom=6),
                        ))
                    # Detect bullet points
                    elif line.startswith("- ") or line.startswith("* "):
                        with me.box(style=me.Style(display="flex", gap=8, margin=me.Margin(left=12))):
                            me.text("•", style=me.Style(color=ACCENT_CYAN))
                            me.text(line[2:], style=me.Style(color=TEXT_SECONDARY, font_size=12))
                    # Detect numbered lists
                    elif len(line) > 2 and line[0].isdigit() and line[1] == ".":
                        with me.box(style=me.Style(display="flex", gap=8, margin=me.Margin(left=12))):
                            me.text(line[:2], style=me.Style(color=ACCENT_CYAN, font_weight="bold"))
                            me.text(line[2:].strip(), style=me.Style(color=TEXT_SECONDARY, font_size=12))
                    # Detect bold text (simple **text** at start)
                    elif line.startswith("**") and "**" in line[2:]:
                        end_idx = line.index("**", 2)
                        bold_text = line[2:end_idx]
                        rest_text = line[end_idx+2:]
                        with me.box(style=me.Style(display="flex", gap=4)):
                            me.text(bold_text, style=me.Style(color=TEXT_PRIMARY, font_weight="bold", font_size=12))
                            if rest_text:
                                me.text(rest_text, style=me.Style(color=TEXT_SECONDARY, font_size=12))
                    # Regular text
                    else:
                        me.text(line, style=me.Style(color=TEXT_SECONDARY, font_size=12, line_height="1.5"))

                # Show truncation notice if response is very long
                if len(lines) > 50:
                    me.text(f"... ({len(lines) - 50} more lines)", style=me.Style(
                        color=TEXT_SECONDARY, font_style="italic", font_size=11,
                        margin=me.Margin(top=8),
                    ))


@me.page(path="/simple", on_load=on_page_load)
def simple_page():
    """Simplified UI with task submission and results log."""
    state = me.state(State)

    with me.box(style=me.Style(
        min_height="100vh",
        background=f"linear-gradient(180deg, {DARK_BG} 0%, #0f172a 100%)",
        padding=me.Padding.all(24),
    )):
        # Header
        with me.box(style=me.Style(margin=me.Margin(bottom=24))):
            me.text("Agent Exchange", style=me.Style(font_size=28, font_weight="bold", color=TEXT_PRIMARY))
            me.text("Submit tasks and view results", style=me.Style(font_size=14, color=TEXT_SECONDARY))

        # Main layout: two columns
        with me.box(style=me.Style(display="flex", gap=24, flex_wrap="wrap")):
            # Left column: Task submission
            with me.box(style=me.Style(
                flex_grow=1,
                min_width=400,
                max_width=500,
            )):
                with me.box(style=me.Style(
                    background=CARD_BG,
                    border_radius=12,
                    padding=me.Padding.all(20),
                    border=me.Border.all(me.BorderSide(width=1, color=CARD_BORDER)),
                )):
                    me.text("Submit New Task", style=me.Style(font_size=18, font_weight="bold", color=TEXT_PRIMARY, margin=me.Margin(bottom=16)))

                    me.text("Describe your legal task:", style=me.Style(color=TEXT_SECONDARY, margin=me.Margin(bottom=8)))
                    me.textarea(
                        value=state.work_description,
                        on_input=on_work_change,
                        placeholder="e.g., Review this NDA for potential risks and compliance issues...",
                        style=me.Style(width="100%", min_height=100, background="#1e293b", color=TEXT_PRIMARY, border_radius=8),
                    )

                    with me.box(style=me.Style(display="flex", gap=16, margin=me.Margin(top=16))):
                        with me.box():
                            me.text("Pages:", style=me.Style(color=TEXT_SECONDARY, font_size=12, margin=me.Margin(bottom=4)))
                            me.input(
                                value=str(state.document_pages),
                                on_input=on_pages_change,
                                type="number",
                                style=me.Style(width=60, background="#1e293b", color=TEXT_PRIMARY),
                            )
                        with me.box():
                            me.text("Strategy:", style=me.Style(color=TEXT_SECONDARY, font_size=12, margin=me.Margin(bottom=4)))
                            me.select(
                                value=state.bid_strategy,
                                options=[
                                    me.SelectOption(label="Balanced", value="balanced"),
                                    me.SelectOption(label="Lowest Price", value="lowest_price"),
                                    me.SelectOption(label="Best Quality", value="best_quality"),
                                ],
                                on_selection_change=on_strategy_change,
                            )

                    if state.error:
                        me.text(state.error, style=me.Style(color="#ef4444", margin=me.Margin(top=12)))

                    me.button(
                        "Run Workflow" if not state.is_running else "Running...",
                        on_click=on_run_workflow,
                        disabled=state.is_running,
                        type="flat",
                        style=me.Style(
                            margin=me.Margin(top=16),
                            background=ACCENT_GREEN if not state.is_running else "#666",
                            color=TEXT_PRIMARY,
                            padding=me.Padding.symmetric(horizontal=24, vertical=12),
                            border_radius=8,
                            width="100%",
                        ),
                    )

                # Live log output
                if state.log_messages:
                    with me.box(style=me.Style(
                        background="#0f172a",
                        border_radius=12,
                        padding=me.Padding.all(16),
                        margin=me.Margin(top=16),
                        max_height=300,
                        overflow_y="auto",
                    )):
                        me.text("Live Output", style=me.Style(font_weight="bold", color=ACCENT_CYAN, margin=me.Margin(bottom=8)))
                        for msg in state.log_messages:
                            me.text(msg, style=me.Style(
                                font_family="monospace",
                                font_size=11,
                                color=TEXT_SECONDARY if msg.startswith("[") else TEXT_PRIMARY,
                                margin=me.Margin(bottom=2),
                            ))

            # Right column: Task results
            with me.box(style=me.Style(flex_grow=2, min_width=500)):
                with me.box(style=me.Style(display="flex", justify_content="space-between", align_items="center", margin=me.Margin(bottom=16))):
                    me.text(f"Task Results ({len(state.task_results)})", style=me.Style(font_size=18, font_weight="bold", color=TEXT_PRIMARY))
                    if state.task_results:
                        me.button(
                            "Clear History",
                            on_click=on_clear_history,
                            type="flat",
                            style=me.Style(
                                background="transparent",
                                color=TEXT_SECONDARY,
                                font_size=12,
                                padding=me.Padding.symmetric(horizontal=12, vertical=6),
                                border=me.Border.all(me.BorderSide(width=1, color=CARD_BORDER)),
                                border_radius=4,
                            ),
                        )

                if not state.task_results:
                    with me.box(style=me.Style(
                        background=CARD_BG,
                        border_radius=12,
                        padding=me.Padding.all(40),
                        text_align="center",
                        border=me.Border.all(me.BorderSide(width=1, color=CARD_BORDER)),
                    )):
                        me.text("No tasks completed yet", style=me.Style(color=TEXT_SECONDARY))
                        me.text("Submit a task to see results here", style=me.Style(color=TEXT_SECONDARY, font_size=12, margin=me.Margin(top=8)))
                else:
                    for task in state.task_results:
                        render_task_result_card(task)

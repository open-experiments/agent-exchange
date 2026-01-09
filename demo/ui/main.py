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
LEGAL_AGENT_A_URL = os.environ.get("LEGAL_AGENT_A_URL", "http://localhost:8100")
LEGAL_AGENT_B_URL = os.environ.get("LEGAL_AGENT_B_URL", "http://localhost:8101")
LEGAL_AGENT_C_URL = os.environ.get("LEGAL_AGENT_C_URL", "http://localhost:8102")


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
    contract_id: str = ""
    agent_response: str = ""
    execution_time_ms: int = 0
    agreed_price: float = 0.0
    platform_fee: float = 0.0
    provider_payout: float = 0.0
    settlement_timestamp: str = ""
    loading: bool = False
    error: str = ""
    # Stats (starting with initial values that increment with each demo run)
    total_transactions: int = 147
    successful_transactions: int = 144
    platform_revenue: float = 2847.50
    avg_response_time: int = 4200
    total_response_time: int = 617400  # 147 * 4200


def simulate_bids(pages: int) -> list[dict]:
    """Simulate bids from the three legal agents."""
    budget_price = 5.0 + (2.0 * pages)
    standard_price = 15.0 + (0.5 * pages)
    premium_price = 30.0 + (0.2 * pages)

    return [
        {
            "provider_id": "legal-agent-a",
            "provider_name": "Budget Legal AI",
            "price": budget_price,
            "confidence": 0.75 + random.uniform(-0.05, 0.05),
            "estimated_minutes": max(3, pages),
            "trust_score": 0.72,
            "tier": "VERIFIED",
        },
        {
            "provider_id": "legal-agent-b",
            "provider_name": "Standard Legal AI",
            "price": standard_price,
            "confidence": 0.85 + random.uniform(-0.05, 0.05),
            "estimated_minutes": max(5, pages * 2),
            "trust_score": 0.85,
            "tier": "TRUSTED",
        },
        {
            "provider_id": "legal-agent-c",
            "provider_name": "Premium Legal AI",
            "price": premium_price,
            "confidence": 0.95 + random.uniform(-0.03, 0.03),
            "estimated_minutes": max(10, pages * 3),
            "trust_score": 0.94,
            "tier": "PREFERRED",
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
    state = me.state(State)
    state.work_description = e.value


def on_pages_change(e: me.InputEvent):
    state = me.state(State)
    try:
        state.document_pages = int(e.value)
    except:
        state.document_pages = 5


def on_strategy_change(e: me.SelectSelectionChangeEvent):
    state = me.state(State)
    state.bid_strategy = e.value


def on_submit_work(e: me.ClickEvent):
    state = me.state(State)
    if not state.work_description.strip():
        state.error = "Please describe your legal task"
        return
    state.error = ""
    state.work_id = f"work-{int(time.time())}"
    state.step = 2
    state.bids = []
    state.bidding_complete = False


def on_simulate_bidding(e: me.ClickEvent):
    state = me.state(State)
    raw_bids = simulate_bids(state.document_pages)
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
        )
        for b in raw_bids
    ]
    state.bidding_complete = True


def on_evaluate_bids(e: me.ClickEvent):
    state = me.state(State)
    bid_dicts = [
        {
            "provider_id": b.provider_id,
            "provider_name": b.provider_name,
            "price": b.price,
            "confidence": b.confidence,
            "estimated_minutes": b.estimated_minutes,
            "trust_score": b.trust_score,
            "tier": b.tier,
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
        state.contract_id = f"contract-{int(time.time())}"
        state.agreed_price = winner.price
        state.step = 4


def on_execute_work(e: me.ClickEvent):
    state = me.state(State)
    state.loading = True
    state.error = ""

    provider_urls = {
        "legal-agent-a": LEGAL_AGENT_A_URL,
        "legal-agent-b": LEGAL_AGENT_B_URL,
        "legal-agent-c": LEGAL_AGENT_C_URL,
    }

    url = provider_urls.get(state.winner_provider_id, LEGAL_AGENT_A_URL)
    response, elapsed = call_agent_a2a(url, state.work_description)

    state.agent_response = response
    state.execution_time_ms = elapsed
    state.loading = False
    state.step = 5


def on_settle(e: me.ClickEvent):
    state = me.state(State)
    state.platform_fee = round(state.agreed_price * 0.15, 2)
    state.provider_payout = round(state.agreed_price - state.platform_fee, 2)
    state.settlement_timestamp = datetime.utcnow().strftime("%Y-%m-%d %H:%M:%S UTC")

    # Update all platform KPIs dynamically
    state.platform_revenue = round(state.platform_revenue + state.platform_fee, 2)
    state.total_transactions += 1
    state.successful_transactions += 1
    state.total_response_time += state.execution_time_ms
    state.avg_response_time = state.total_response_time // state.total_transactions

    state.step = 6


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
    state.contract_id = ""
    state.agent_response = ""
    state.execution_time_ms = 0
    state.agreed_price = 0.0
    state.platform_fee = 0.0
    state.provider_payout = 0.0
    state.settlement_timestamp = ""
    state.loading = False
    state.error = ""


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
        ("SETTLE", "Payment", "#ec4899", 6),
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


def render_provider_card(name: str, tier: str, status: str, trust: float, color: str):
    """Render a provider card."""
    tier_colors = {"VERIFIED": ACCENT_ORANGE, "TRUSTED": ACCENT_BLUE, "PREFERRED": ACCENT_GREEN}

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
                me.text(status, style=me.Style(font_size=12, color=color))


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
        "Proceed to Settlement →",
        on_click=on_settle,
        style=me.Style(background="#ec4899", color=TEXT_PRIMARY, padding=me.Padding.symmetric(horizontal=24, vertical=12), border_radius=8),
    )


def render_step_6():
    """Step 6: Settlement - Transaction Receipt."""
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


@me.page(path="/")
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
            render_kpi_card(f"{state.total_transactions}", "Total Transactions", ACCENT_CYAN)
            render_kpi_card(f"${state.platform_revenue:.0f}", "Platform Revenue", ACCENT_GREEN)
            render_kpi_card(f"{state.avg_response_time}ms", "Avg Response Time", ACCENT_ORANGE)
            success_rate = (state.successful_transactions / state.total_transactions * 100) if state.total_transactions > 0 else 0
            render_kpi_card(f"{success_rate:.1f}%", "Success Rate", ACCENT_PURPLE)

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
                me.text("Registered Providers", style=me.Style(
                    font_size=14,
                    font_weight="bold",
                    color=TEXT_PRIMARY,
                    margin=me.Margin(bottom=12),
                ))

                with me.box(style=me.Style(display="flex", flex_direction="column", gap=12)):
                    render_provider_card("Budget Legal AI", "VERIFIED", "Online", 0.72, ACCENT_GREEN)
                    render_provider_card("Standard Legal AI", "TRUSTED", "Online", 0.85, ACCENT_GREEN)
                    render_provider_card("Premium Legal AI", "PREFERRED", "Online", 0.94, ACCENT_GREEN)

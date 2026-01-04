"""AEX + A2A Demo UI using Mesop."""

import json
import os
import httpx
import mesop as me


# Configuration - read from environment or use defaults
ORCHESTRATOR_URL = os.environ.get("ORCHESTRATOR_URL", "http://localhost:8103")
LEGAL_AGENT_A_URL = os.environ.get("LEGAL_AGENT_A_URL", "http://localhost:8100")
LEGAL_AGENT_B_URL = os.environ.get("LEGAL_AGENT_B_URL", "http://localhost:8101")
LEGAL_AGENT_C_URL = os.environ.get("LEGAL_AGENT_C_URL", "http://localhost:8102")


from dataclasses import field


@me.stateclass
class State:
    """Application state."""
    user_input: str = ""
    response: str = ""
    loading: bool = False
    selected_agent: str = "orchestrator"
    agent_status: dict = field(default_factory=dict)
    error: str = ""


def get_agent_url(agent: str) -> str:
    """Get URL for selected agent."""
    urls = {
        "orchestrator": ORCHESTRATOR_URL,
        "legal_a": LEGAL_AGENT_A_URL,
        "legal_b": LEGAL_AGENT_B_URL,
        "legal_c": LEGAL_AGENT_C_URL,
    }
    return urls.get(agent, ORCHESTRATOR_URL)


def call_agent(url: str, message: str) -> str:
    """Call an agent via A2A protocol."""
    a2a_url = f"{url}/a2a"

    payload = {
        "jsonrpc": "2.0",
        "method": "message/send",
        "id": "demo-1",
        "params": {
            "message": {
                "role": "user",
                "parts": [{"type": "text", "text": message}],
            }
        },
    }

    try:
        with httpx.Client(timeout=120.0) as client:
            resp = client.post(a2a_url, json=payload)
            resp.raise_for_status()
            data = resp.json()

            if "error" in data:
                return f"Error: {data['error'].get('message', 'Unknown error')}"

            result = data.get("result", {})
            history = result.get("history", [])

            # Extract agent response
            for msg in reversed(history):
                if msg.get("role") == "agent":
                    parts = msg.get("parts", [])
                    for part in parts:
                        if part.get("type") == "text":
                            return part.get("text", "No text response")

            return "No response from agent"

    except httpx.HTTPError as e:
        return f"HTTP Error: {str(e)}"
    except Exception as e:
        return f"Error: {str(e)}"


def check_agent_health(url: str) -> bool:
    """Check if an agent is healthy."""
    try:
        with httpx.Client(timeout=5.0) as client:
            resp = client.get(f"{url}/health")
            return resp.status_code == 200
    except Exception:
        return False


def on_input_change(e: me.InputEvent):
    """Handle input change."""
    state = me.state(State)
    state.user_input = e.value


def on_agent_select(e: me.SelectSelectionChangeEvent):
    """Handle agent selection."""
    state = me.state(State)
    state.selected_agent = e.value


def on_submit(e: me.ClickEvent):
    """Handle form submission."""
    state = me.state(State)
    if not state.user_input.strip():
        state.error = "Please enter a message"
        return

    state.loading = True
    state.error = ""
    state.response = ""

    # Call selected agent
    url = get_agent_url(state.selected_agent)
    state.response = call_agent(url, state.user_input)
    state.loading = False


def on_check_status(e: me.ClickEvent):
    """Check status of all agents."""
    state = me.state(State)
    agents = {
        "Orchestrator": ORCHESTRATOR_URL,
        "Budget Legal": LEGAL_AGENT_A_URL,
        "Standard Legal": LEGAL_AGENT_B_URL,
        "Premium Legal": LEGAL_AGENT_C_URL,
    }

    status = {}
    for name, url in agents.items():
        status[name] = "Online" if check_agent_health(url) else "Offline"

    state.agent_status = status


def on_clear(e: me.ClickEvent):
    """Clear the response."""
    state = me.state(State)
    state.response = ""
    state.error = ""


@me.page(path="/")
def home():
    """Main demo page."""
    state = me.state(State)

    with me.box(style=me.Style(
        padding=me.Padding.all(24),
        max_width=1200,
        margin=me.Margin.symmetric(horizontal="auto"),
    )):
        # Header
        me.text(
            "Agent Exchange + A2A Demo",
            style=me.Style(
                font_size=32,
                font_weight="bold",
                margin=me.Margin(bottom=8),
            ),
        )
        me.text(
            "Intelligent agent discovery and orchestration",
            style=me.Style(
                font_size=16,
                color="#666",
                margin=me.Margin(bottom=24),
            ),
        )

        # Agent status section
        with me.box(style=me.Style(
            background="#f5f5f5",
            padding=me.Padding.all(16),
            border_radius=8,
            margin=me.Margin(bottom=24),
        )):
            with me.box(style=me.Style(
                display="flex",
                justify_content="space-between",
                align_items="center",
                margin=me.Margin(bottom=12),
            )):
                me.text("Agent Status", style=me.Style(font_weight="bold"))
                me.button("Check Status", on_click=on_check_status, type="flat")

            if state.agent_status:
                with me.box(style=me.Style(display="flex", gap=16)):
                    for name, status in state.agent_status.items():
                        color = "#4caf50" if status == "Online" else "#f44336"
                        with me.box(style=me.Style(
                            padding=me.Padding.all(8),
                            background="white",
                            border_radius=4,
                            min_width=120,
                        )):
                            me.text(name, style=me.Style(font_size=12, color="#666"))
                            me.text(status, style=me.Style(color=color, font_weight="bold"))

        # Input section
        with me.box(style=me.Style(margin=me.Margin(bottom=24))):
            me.text("Select Agent:", style=me.Style(margin=me.Margin(bottom=8)))
            me.select(
                value=state.selected_agent,
                options=[
                    me.SelectOption(label="Orchestrator (Multi-agent)", value="orchestrator"),
                    me.SelectOption(label="Budget Legal ($5 + $2/page)", value="legal_a"),
                    me.SelectOption(label="Standard Legal ($15 + $0.50/page)", value="legal_b"),
                    me.SelectOption(label="Premium Legal ($30 + $0.20/page)", value="legal_c"),
                ],
                on_selection_change=on_agent_select,
                style=me.Style(width=300, margin=me.Margin(bottom=16)),
            )

            me.text("Your Request:", style=me.Style(margin=me.Margin(bottom=8)))
            me.textarea(
                value=state.user_input,
                on_input=on_input_change,
                placeholder="e.g., Review this NDA for potential risks. Focus on liability clauses, termination conditions, and intellectual property provisions.",
                style=me.Style(width="100%", min_height=100),
            )

            with me.box(style=me.Style(
                display="flex",
                gap=12,
                margin=me.Margin(top=16),
            )):
                me.button(
                    "Send Request",
                    on_click=on_submit,
                    disabled=state.loading,
                    type="raised",
                )
                me.button(
                    "Clear",
                    on_click=on_clear,
                    type="flat",
                )

            if state.loading:
                me.progress_spinner()

        # Error display
        if state.error:
            with me.box(style=me.Style(
                background="#ffebee",
                color="#c62828",
                padding=me.Padding.all(12),
                border_radius=4,
                margin=me.Margin(bottom=16),
            )):
                me.text(state.error)

        # Response section
        if state.response:
            with me.box(style=me.Style(
                background="white",
                border=me.Border.all(me.BorderSide(width=1, color="#e0e0e0")),
                border_radius=8,
                padding=me.Padding.all(20),
            )):
                me.text(
                    "Response",
                    style=me.Style(
                        font_weight="bold",
                        font_size=18,
                        margin=me.Margin(bottom=16),
                    ),
                )
                me.markdown(state.response)

        # Demo scenarios
        with me.box(style=me.Style(margin=me.Margin(top=32))):
            me.text(
                "Example Scenarios",
                style=me.Style(
                    font_weight="bold",
                    font_size=18,
                    margin=me.Margin(bottom=16),
                ),
            )

            scenarios = [
                {
                    "title": "Multi-Agent Contract Analysis",
                    "description": "I need a comprehensive review of this partnership agreement. Compare different analysis approaches and recommend the best value option.",
                    "agent": "orchestrator",
                },
                {
                    "title": "Quick NDA Review (Budget)",
                    "description": "Review this NDA for potential risks: [Standard NDA with 2-year term, broad definition of confidential information, and no carve-outs for public information]",
                    "agent": "legal_a",
                },
                {
                    "title": "GDPR Compliance Check (Standard)",
                    "description": "Check if our privacy policy complies with GDPR. We collect email, name, and usage data for analytics.",
                    "agent": "legal_b",
                },
                {
                    "title": "Complex IP Agreement (Premium)",
                    "description": "Provide exhaustive analysis of this intellectual property licensing agreement including jurisdiction-specific considerations and negotiation recommendations.",
                    "agent": "legal_c",
                },
            ]

            with me.box(style=me.Style(
                display="grid",
                grid_template_columns="repeat(2, 1fr)",
                gap=16,
            )):
                for scenario in scenarios:
                    with me.box(style=me.Style(
                        background="#f9f9f9",
                        padding=me.Padding.all(16),
                        border_radius=8,
                        cursor="pointer",
                    )):
                        me.text(
                            scenario["title"],
                            style=me.Style(font_weight="bold", margin=me.Margin(bottom=8)),
                        )
                        me.text(
                            scenario["description"][:100] + "..." if len(scenario["description"]) > 100 else scenario["description"],
                            style=me.Style(font_size=14, color="#666"),
                        )


# App is started via mesop CLI: mesop main.py --port=8501

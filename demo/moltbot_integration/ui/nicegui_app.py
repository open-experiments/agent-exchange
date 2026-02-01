"""Token Banking Dashboard - NiceGUI real-time UI for Moltbot demo with AP2 support."""

import asyncio
import json
import os
from dataclasses import dataclass
from datetime import datetime
from typing import Optional

import aiohttp
from nicegui import ui, app

# Configuration
TOKEN_BANK_URL = os.environ.get("TOKEN_BANK_URL", "http://aex-token-bank:8094")
AEX_REGISTRY_URL = os.environ.get("AEX_REGISTRY_URL", "http://aex-provider-registry:8080")

# Agent endpoints for health checks
AGENT_ENDPOINTS = {
    "moltbot-researcher": os.environ.get("RESEARCHER_URL", "http://moltbot-researcher:8095"),
    "moltbot-writer": os.environ.get("WRITER_URL", "http://moltbot-writer:8096"),
    "moltbot-analyst": os.environ.get("ANALYST_URL", "http://moltbot-analyst:8097"),
}


@dataclass
class Wallet:
    agent_id: str
    agent_name: str
    balance: float
    token_type: str


@dataclass
class Transaction:
    id: str
    from_wallet: str
    to_wallet: str
    amount: float
    description: str
    reference: str
    created_at: str


@dataclass
class AP2Mandate:
    id: str
    type: str  # intent, cart, payment
    consumer_id: str
    provider_id: str
    amount: float
    status: str
    created_at: str


async def fetch_wallets() -> list[Wallet]:
    """Fetch all wallets from Token Bank."""
    try:
        async with aiohttp.ClientSession() as session:
            async with session.get(f"{TOKEN_BANK_URL}/wallets") as resp:
                if resp.status == 200:
                    data = await resp.json()
                    return [
                        Wallet(
                            agent_id=w["agent_id"],
                            agent_name=w["agent_name"],
                            balance=w["balance"],
                            token_type=w["token_type"],
                        )
                        for w in data.get("wallets", [])
                    ]
    except Exception as e:
        print(f"Error fetching wallets: {e}")
    return []


async def fetch_transactions(agent_id: str) -> list[Transaction]:
    """Fetch transaction history for an agent."""
    try:
        async with aiohttp.ClientSession() as session:
            async with session.get(f"{TOKEN_BANK_URL}/wallets/{agent_id}/history") as resp:
                if resp.status == 200:
                    data = await resp.json()
                    return [
                        Transaction(
                            id=tx["id"],
                            from_wallet=tx["from_wallet"],
                            to_wallet=tx["to_wallet"],
                            amount=tx["amount"],
                            description=tx.get("description", ""),
                            reference=tx.get("reference", ""),
                            created_at=tx.get("created_at", ""),
                        )
                        for tx in data.get("transactions", [])
                    ]
    except Exception as e:
        print(f"Error fetching transactions: {e}")
    return []


async def fetch_ap2_capabilities() -> dict:
    """Fetch AP2 capabilities from Token Bank."""
    try:
        async with aiohttp.ClientSession() as session:
            async with session.get(f"{TOKEN_BANK_URL}/ap2/capabilities") as resp:
                if resp.status == 200:
                    return await resp.json()
    except Exception as e:
        print(f"Error fetching AP2 capabilities: {e}")
    return {}


async def fetch_ap2_mandates(agent_id: str) -> list[AP2Mandate]:
    """Fetch AP2 mandates for an agent."""
    try:
        async with aiohttp.ClientSession() as session:
            async with session.get(f"{TOKEN_BANK_URL}/ap2/mandates/{agent_id}") as resp:
                if resp.status == 200:
                    data = await resp.json()
                    return [
                        AP2Mandate(
                            id=m["id"],
                            type=m["type"],
                            consumer_id=m["consumer_id"],
                            provider_id=m["provider_id"],
                            amount=m["amount"],
                            status=m["status"],
                            created_at=m.get("created_at", ""),
                        )
                        for m in data.get("mandates", [])
                    ]
    except Exception as e:
        print(f"Error fetching AP2 mandates: {e}")
    return []


async def check_agent_health(agent_id: str) -> bool:
    """Check if an agent is healthy."""
    endpoint = AGENT_ENDPOINTS.get(agent_id)
    if not endpoint:
        return False
    try:
        async with aiohttp.ClientSession() as session:
            async with session.get(f"{endpoint}/health", timeout=aiohttp.ClientTimeout(total=2)) as resp:
                return resp.status == 200
    except:
        return False


async def execute_transfer(from_agent: str, to_agent: str, amount: float, description: str) -> bool:
    """Execute a token transfer."""
    try:
        async with aiohttp.ClientSession() as session:
            payload = {
                "from_agent_id": from_agent,
                "to_agent_id": to_agent,
                "amount": amount,
                "description": description,
            }
            async with session.post(f"{TOKEN_BANK_URL}/transfers", json=payload) as resp:
                return resp.status == 200
    except Exception as e:
        print(f"Error executing transfer: {e}")
    return False


async def execute_ap2_payment(
    consumer_id: str, provider_id: str, amount: float, description: str
) -> tuple[bool, Optional[dict]]:
    """Execute a payment using AP2 mandate chain."""
    try:
        async with aiohttp.ClientSession() as session:
            payload = {
                "consumer_id": consumer_id,
                "provider_id": provider_id,
                "amount": amount,
                "description": description,
            }
            async with session.post(f"{TOKEN_BANK_URL}/ap2/process-chain", json=payload) as resp:
                data = await resp.json()
                return data.get("success", False), data.get("receipt")
    except Exception as e:
        print(f"Error executing AP2 payment: {e}")
    return False, None


async def deposit_tokens(agent_id: str, amount: float, description: str) -> bool:
    """Deposit tokens to an agent's wallet."""
    try:
        async with aiohttp.ClientSession() as session:
            payload = {"amount": amount, "description": description}
            async with session.post(f"{TOKEN_BANK_URL}/wallets/{agent_id}/deposit", json=payload) as resp:
                return resp.status == 200
    except Exception as e:
        print(f"Error depositing: {e}")
    return False


def create_ui():
    """Create the Token Banking Dashboard UI with AP2 support."""

    # State
    wallets_data: list[Wallet] = []
    selected_agent: Optional[str] = None
    transactions_data: list[Transaction] = []
    ap2_mandates: list[AP2Mandate] = []
    ap2_capabilities: dict = {}
    current_tab = "wallets"

    # Dark theme
    ui.dark_mode().enable()

    # Header
    with ui.header().classes('bg-blue-900 text-white'):
        with ui.row().classes('items-center gap-4'):
            ui.label('🏦 Token Banking Dashboard').classes('text-2xl font-bold')
            ui.label('Moltbot + AEX + AP2 Integration').classes('text-sm opacity-75')
        with ui.row().classes('ml-auto items-center gap-2'):
            ap2_badge = ui.badge('AP2 Enabled').props('color=green')
            refresh_btn = ui.button('Refresh', on_click=lambda: refresh_data()).props('flat color=white')

    # Tabs
    with ui.tabs().classes('w-full') as tabs:
        wallets_tab = ui.tab('Wallets')
        ap2_tab = ui.tab('AP2 Payments')
        history_tab = ui.tab('Transaction History')

    with ui.tab_panels(tabs, value=wallets_tab).classes('w-full'):
        # ==========================================
        # WALLETS TAB
        # ==========================================
        with ui.tab_panel(wallets_tab).classes('p-4'):
            with ui.row().classes('w-full gap-4'):
                # Left panel - Wallets
                with ui.card().classes('w-1/3'):
                    ui.label('Agent Wallets').classes('text-xl font-bold mb-4')

                    wallets_container = ui.column().classes('w-full gap-2')

                    @ui.refreshable
                    def render_wallets():
                        if not wallets_data:
                            ui.label('No wallets found. Start some agents!').classes('text-gray-500')
                        else:
                            for wallet in wallets_data:
                                with ui.card().classes('w-full cursor-pointer hover:bg-blue-900/20').on(
                                    'click', lambda w=wallet: select_agent(w.agent_id)
                                ):
                                    with ui.row().classes('justify-between items-center'):
                                        with ui.column():
                                            ui.label(wallet.agent_name).classes('font-bold')
                                            ui.label(wallet.agent_id).classes('text-xs text-gray-500')
                                        with ui.column().classes('items-end'):
                                            ui.label(f'{wallet.balance:.2f}').classes('text-2xl font-bold text-green-400')
                                            ui.label(wallet.token_type).classes('text-xs text-gray-500')

                    render_wallets()  # Initial render

                # Middle panel - Quick Actions
                with ui.card().classes('w-1/3'):
                    ui.label('Quick Actions').classes('text-xl font-bold mb-4')

                    # Transfer section
                    ui.label('Direct Transfer').classes('font-bold')
                    from_select = ui.select([], label='From Agent').classes('w-full')
                    to_select = ui.select([], label='To Agent').classes('w-full')
                    amount_input = ui.number(label='Amount', value=10, min=1).classes('w-full')
                    desc_input = ui.input(label='Description').classes('w-full')

                    async def do_transfer():
                        if from_select.value and to_select.value and amount_input.value:
                            success = await execute_transfer(
                                from_select.value,
                                to_select.value,
                                amount_input.value,
                                desc_input.value or "Manual transfer"
                            )
                            if success:
                                ui.notify('Transfer successful!', type='positive')
                                await refresh_data()
                            else:
                                ui.notify('Transfer failed!', type='negative')

                    ui.button('Execute Transfer', on_click=do_transfer).classes('w-full mt-2')

                    ui.separator().classes('my-4')

                    # Deposit section
                    ui.label('Deposit Tokens').classes('font-bold')
                    deposit_agent = ui.select([], label='Agent').classes('w-full')
                    deposit_amount = ui.number(label='Amount', value=50, min=1).classes('w-full')

                    async def do_deposit():
                        if deposit_agent.value and deposit_amount.value:
                            success = await deposit_tokens(
                                deposit_agent.value,
                                deposit_amount.value,
                                "Manual deposit from UI"
                            )
                            if success:
                                ui.notify('Deposit successful!', type='positive')
                                await refresh_data()
                            else:
                                ui.notify('Deposit failed!', type='negative')

                    ui.button('Deposit', on_click=do_deposit).classes('w-full mt-2')

                # Right panel - Stats
                with ui.card().classes('w-1/3'):
                    ui.label('System Stats').classes('text-xl font-bold mb-4')

                    stats_container = ui.column().classes('w-full gap-4')

                    @ui.refreshable
                    def render_stats():
                        total_balance = sum(w.balance for w in wallets_data)
                        with ui.card().classes('w-full bg-blue-900/20'):
                            ui.label('Total Agents').classes('text-sm text-gray-400')
                            ui.label(f'{len(wallets_data)}').classes('text-3xl font-bold text-white')
                        with ui.card().classes('w-full bg-green-900/20'):
                            ui.label('Total Token Supply').classes('text-sm text-gray-400')
                            ui.label(f'{total_balance:.2f} AEX').classes('text-3xl font-bold text-green-400')
                        with ui.card().classes('w-full bg-purple-900/20'):
                            ui.label('Payment Protocol').classes('text-sm text-gray-400')
                            ui.label('AP2 (Google)').classes('text-xl font-bold text-purple-400')

                    render_stats()  # Initial render

        # ==========================================
        # AP2 PAYMENTS TAB
        # ==========================================
        with ui.tab_panel(ap2_tab).classes('p-4'):
            with ui.row().classes('w-full gap-4'):
                # Left panel - AP2 Payment Form
                with ui.card().classes('w-1/2'):
                    ui.label('AP2 Payment Flow').classes('text-xl font-bold mb-4')
                    ui.label('Execute payment using full AP2 mandate chain:').classes('text-sm text-gray-400 mb-2')
                    ui.label('IntentMandate → CartMandate → PaymentMandate → Receipt').classes('text-xs text-purple-400 mb-4')

                    ap2_from = ui.select([], label='Consumer (Payer)').classes('w-full')
                    ap2_to = ui.select([], label='Provider (Payee)').classes('w-full')
                    ap2_amount = ui.number(label='Amount (AEX)', value=10, min=1).classes('w-full')
                    ap2_desc = ui.input(label='Service Description', value='Agent service payment').classes('w-full')

                    receipt_container = ui.column().classes('w-full mt-4')

                    async def do_ap2_payment():
                        if ap2_from.value and ap2_to.value and ap2_amount.value:
                            receipt_container.clear()
                            with receipt_container:
                                with ui.card().classes('w-full bg-yellow-900/20'):
                                    ui.spinner('dots')
                                    ui.label('Processing AP2 mandate chain...').classes('text-yellow-400')

                            success, receipt = await execute_ap2_payment(
                                ap2_from.value,
                                ap2_to.value,
                                ap2_amount.value,
                                ap2_desc.value or "AP2 Payment"
                            )

                            receipt_container.clear()
                            with receipt_container:
                                if success and receipt:
                                    with ui.card().classes('w-full bg-green-900/20'):
                                        ui.label('✅ Payment Successful!').classes('text-xl font-bold text-green-400')
                                        ui.separator()
                                        ui.label(f"Payment ID: {receipt.get('payment_id', 'N/A')}").classes('text-sm')
                                        ui.label(f"Mandate ID: {receipt.get('payment_mandate_id', 'N/A')}").classes('text-sm')
                                        ui.label(f"Amount: {receipt.get('amount', {}).get('value', '?')} {receipt.get('amount', {}).get('currency', 'AEX')}").classes('text-sm')
                                        ui.label(f"Status: {receipt.get('payment_status', 'unknown')}").classes('text-sm')
                                    ui.notify('AP2 payment completed!', type='positive')
                                else:
                                    with ui.card().classes('w-full bg-red-900/20'):
                                        ui.label('❌ Payment Failed').classes('text-xl font-bold text-red-400')
                                        if receipt and receipt.get('error'):
                                            ui.label(f"Error: {receipt['error'].get('message', 'Unknown error')}").classes('text-sm text-red-300')
                                    ui.notify('AP2 payment failed!', type='negative')

                            await refresh_data()

                    ui.button('Execute AP2 Payment', on_click=do_ap2_payment, color='purple').classes('w-full mt-4')

                # Right panel - AP2 Capabilities and Flow
                with ui.card().classes('w-1/2'):
                    ui.label('AP2 Provider Capabilities').classes('text-xl font-bold mb-4')

                    caps_container = ui.column().classes('w-full gap-2')

                    @ui.refreshable
                    def render_ap2_caps():
                        if ap2_capabilities:
                            with ui.card().classes('w-full'):
                                ui.label(f"Provider: {ap2_capabilities.get('provider_name', 'N/A')}").classes('font-bold')
                                ui.label(f"Provider ID: {ap2_capabilities.get('provider_id', 'N/A')}").classes('text-xs text-gray-500')
                            with ui.row().classes('gap-2'):
                                for method in ap2_capabilities.get('supported_methods', []):
                                    ui.badge(method).props('color=purple')
                            ui.label(f"Token Type: {ap2_capabilities.get('token_type', 'AEX')}").classes('text-sm')
                            ui.label(f"Fraud Protection: {ap2_capabilities.get('fraud_protection', 'N/A')}").classes('text-sm')
                            ui.label(f"Version: {ap2_capabilities.get('version', 'N/A')}").classes('text-sm')
                        else:
                            ui.label('Loading AP2 capabilities...').classes('text-gray-500')

                    render_ap2_caps()  # Initial render

                    ui.separator().classes('my-4')

                    # AP2 Flow Diagram
                    ui.label('AP2 Mandate Chain').classes('text-lg font-bold mb-2')
                    with ui.card().classes('w-full bg-gray-800 p-4'):
                        ui.mermaid('''
                            sequenceDiagram
                                participant C as Consumer
                                participant TB as Token Bank
                                participant P as Provider
                                C->>TB: 1. IntentMandate
                                TB->>TB: 2. CartMandate
                                C->>TB: 3. PaymentMandate
                                TB->>TB: 4. Execute Transfer
                                TB-->>C: 5. PaymentReceipt
                                TB-->>P: Tokens Credited
                        ''')

        # ==========================================
        # TRANSACTION HISTORY TAB
        # ==========================================
        with ui.tab_panel(history_tab).classes('p-4'):
            with ui.row().classes('w-full gap-4'):
                # Agent selector
                with ui.card().classes('w-1/4'):
                    ui.label('Select Agent').classes('text-xl font-bold mb-4')
                    history_agent_select = ui.select([], label='Agent').classes('w-full')

                    async def on_agent_change():
                        nonlocal selected_agent
                        selected_agent = history_agent_select.value
                        await load_transactions()
                        await load_mandates()

                    history_agent_select.on('change', on_agent_change)

                # Transaction list
                with ui.card().classes('w-3/4'):
                    ui.label('Transactions & Mandates').classes('text-xl font-bold mb-4')

                    with ui.tabs().classes('w-full') as sub_tabs:
                        tx_sub_tab = ui.tab('Transactions')
                        mandate_sub_tab = ui.tab('AP2 Mandates')

                    with ui.tab_panels(sub_tabs, value=tx_sub_tab).classes('w-full'):
                        # Transactions
                        with ui.tab_panel(tx_sub_tab):
                            transactions_container = ui.column().classes('w-full gap-2 max-h-96 overflow-auto')

                            @ui.refreshable
                            def render_transactions():
                                if not selected_agent:
                                    ui.label('Select an agent to view transactions').classes('text-gray-500')
                                elif not transactions_data:
                                    ui.label('No transactions yet').classes('text-gray-500')
                                else:
                                    for tx in reversed(transactions_data[-20:]):  # Show last 20
                                        with ui.card().classes('w-full p-3'):
                                            with ui.row().classes('justify-between items-center'):
                                                direction = "OUT" if tx.from_wallet == selected_agent else "IN"
                                                color = "red" if direction == "OUT" else "green"
                                                ui.badge(direction).props(f'color={color}')
                                                ui.label(f'{tx.amount:.2f} AEX').classes(f'text-{color}-400 font-bold')
                                            ui.label(f'{tx.from_wallet} → {tx.to_wallet}').classes('text-sm text-gray-400')
                                            if tx.reference and tx.reference.startswith('ap2-'):
                                                ui.badge('AP2').props('color=purple outline')
                                            if tx.description:
                                                ui.label(tx.description).classes('text-xs text-gray-500')
                                            ui.label(tx.created_at).classes('text-xs text-gray-600')

                            render_transactions()  # Initial render

                        # AP2 Mandates
                        with ui.tab_panel(mandate_sub_tab):
                            mandates_container = ui.column().classes('w-full gap-2 max-h-96 overflow-auto')

                            @ui.refreshable
                            def render_mandates():
                                if not selected_agent:
                                    ui.label('Select an agent to view mandates').classes('text-gray-500')
                                elif not ap2_mandates:
                                    ui.label('No AP2 mandates yet').classes('text-gray-500')
                                else:
                                    for mandate in reversed(ap2_mandates[-20:]):
                                        type_colors = {
                                            'intent': 'blue',
                                            'cart': 'orange',
                                            'payment': 'purple',
                                        }
                                        status_colors = {
                                            'pending': 'yellow',
                                            'completed': 'green',
                                            'used': 'gray',
                                            'failed': 'red',
                                        }
                                        with ui.card().classes('w-full p-3'):
                                            with ui.row().classes('justify-between items-center'):
                                                ui.badge(mandate.type.upper()).props(f'color={type_colors.get(mandate.type, "gray")}')
                                                ui.badge(mandate.status).props(f'color={status_colors.get(mandate.status, "gray")}')
                                                ui.label(f'{mandate.amount:.2f} AEX').classes('font-bold')
                                            ui.label(f'{mandate.consumer_id} → {mandate.provider_id}').classes('text-sm text-gray-400')
                                            ui.label(f'ID: {mandate.id[:12]}...').classes('text-xs text-gray-500')

                            render_mandates()  # Initial render

    # Footer with stats
    with ui.footer().classes('bg-gray-800'):
        footer_stats = ui.row().classes('w-full justify-around')

        @ui.refreshable
        def render_footer_stats():
            total_balance = sum(w.balance for w in wallets_data)
            ui.label(f'Agents: {len(wallets_data)}').classes('text-white')
            ui.label(f'Total Supply: {total_balance:.2f} AEX').classes('text-white')
            ui.label(f'AP2 Mandates: {len(ap2_mandates)}').classes('text-white')

        render_footer_stats()  # Initial render

    # Helper functions
    def select_agent(agent_id: str):
        nonlocal selected_agent
        selected_agent = agent_id
        history_agent_select.value = agent_id
        asyncio.create_task(load_transactions())
        asyncio.create_task(load_mandates())

    async def load_transactions():
        nonlocal transactions_data
        if selected_agent:
            transactions_data = await fetch_transactions(selected_agent)
            render_transactions.refresh()

    async def load_mandates():
        nonlocal ap2_mandates
        if selected_agent:
            ap2_mandates = await fetch_ap2_mandates(selected_agent)
            render_mandates.refresh()

    async def refresh_data():
        nonlocal wallets_data, ap2_capabilities
        wallets_data = await fetch_wallets()
        ap2_capabilities = await fetch_ap2_capabilities()

        # Update selects
        agent_options = {w.agent_id: w.agent_name for w in wallets_data}
        from_select.options = agent_options
        to_select.options = agent_options
        deposit_agent.options = agent_options
        ap2_from.options = agent_options
        ap2_to.options = agent_options
        history_agent_select.options = agent_options

        render_wallets.refresh()
        render_stats.refresh()
        render_ap2_caps.refresh()
        render_footer_stats.refresh()

        if selected_agent:
            await load_transactions()
            await load_mandates()

    # Auto-refresh every 5 seconds
    async def auto_refresh():
        while True:
            await asyncio.sleep(5)
            try:
                await refresh_data()
            except Exception as e:
                print(f"Auto-refresh error: {e}")

    # Initialize on first page load using a one-shot timer
    async def on_startup():
        await refresh_data()
        asyncio.create_task(auto_refresh())

    # Use timer for initial load (runs once after 0.1s)
    ui.timer(0.1, on_startup, once=True)


# Create the UI
create_ui()

# Run the app
if __name__ in {"__main__", "__mp_main__"}:
    port = int(os.environ.get("PORT", "8503"))
    ui.run(
        host="0.0.0.0",
        port=port,
        title="Token Banking Dashboard",
        favicon="🏦",
        dark=True,
        reload=False,
    )

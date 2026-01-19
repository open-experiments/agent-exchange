#!/usr/bin/env python3
"""ContractPay Payment Agent - Main entry point."""

import asyncio
import logging
import os
import sys

# Add parent directory to path for common imports
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from common.a2a_server import A2AServer
from common.config import load_config
from agent import ContractPayAgent

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
)
logger = logging.getLogger(__name__)


async def main():
    """Start the ContractPay payment agent server."""
    # Load configuration
    config_path = os.environ.get("CONFIG_PATH", "config.yaml")
    config = load_config(config_path)

    logger.info(f"Starting {config.name}")

    # Create agent
    agent = ContractPayAgent(config=config)

    # Generate agent card
    hostname = os.environ.get("AGENT_HOSTNAME", "localhost")
    base_url = f"http://{hostname}:{config.server.port}"

    agent_card = config.get_agent_card(base_url)

    # Create A2A server
    server = A2AServer(
        agent_card=agent_card,
        handler=agent,
        require_auth=False,
    )

    # Register with AEX if enabled
    if config.aex.enabled and config.aex.auto_register:
        try:
            await agent.register_with_aex(base_url)
        except Exception as e:
            logger.warning(f"Could not register with AEX: {e}")

    # Run server
    logger.info(f"Agent Card: {base_url}/.well-known/agent-card.json")
    logger.info(f"A2A Endpoint: {base_url}/a2a")
    logger.info(f"Payment provider ready - GET CASHBACK on contract work!")
    await server.run_async(host=config.server.host, port=config.server.port)


if __name__ == "__main__":
    asyncio.run(main())

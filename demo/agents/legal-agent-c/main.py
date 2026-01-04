#!/usr/bin/env python3
"""Legal Agent C (Premium) - Main entry point."""

import asyncio
import logging
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from common.a2a_server import A2AServer
from common.config import load_config
from agent import LegalAgentC

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
)
logger = logging.getLogger(__name__)


async def main():
    """Start the Legal Agent C (Premium) server."""
    config_path = os.environ.get("CONFIG_PATH", "config.yaml")
    config = load_config(config_path)

    logger.info(f"Starting {config.name} (Premium Tier)")
    logger.info(f"Skills: {[s.id for s in config.skills]}")
    logger.info(f"Pricing: ${config.aex.base_rate}/request")

    agent = LegalAgentC(config=config)

    # Use AGENT_HOSTNAME env var for Docker, fallback to localhost
    hostname = os.environ.get("AGENT_HOSTNAME", "localhost")
    base_url = f"http://{hostname}:{config.server.port}"

    agent_card = config.get_agent_card(base_url)

    server = A2AServer(
        agent_card=agent_card,
        handler=agent,
        require_auth=False,
    )

    if config.aex.enabled and config.aex.auto_register:
        try:
            await agent.register_with_aex(base_url)
        except Exception as e:
            logger.warning(f"Could not register with AEX: {e}")

    logger.info(f"Agent Card: {base_url}/.well-known/agent-card.json")
    logger.info(f"A2A Endpoint: {base_url}/a2a")
    await server.run_async(host=config.server.host, port=config.server.port)


if __name__ == "__main__":
    asyncio.run(main())

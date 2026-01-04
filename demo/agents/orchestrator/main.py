#!/usr/bin/env python3
"""Orchestrator Agent - Main entry point."""

import asyncio
import logging
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from common.a2a_server import A2AServer
from common.config import load_config
from agent import OrchestratorAgent

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
)
logger = logging.getLogger(__name__)


async def main():
    """Start the Orchestrator Agent server."""
    config_path = os.environ.get("CONFIG_PATH", "config.yaml")
    config = load_config(config_path)

    logger.info(f"Starting {config.name}")
    logger.info(f"AEX Gateway: {config.aex.gateway_url}")

    agent = OrchestratorAgent(config=config)

    base_url = f"http://{config.server.host}:{config.server.port}"
    if config.server.host == "0.0.0.0":
        base_url = f"http://localhost:{config.server.port}"

    agent_card = config.get_agent_card(base_url)

    server = A2AServer(
        agent_card=agent_card,
        handler=agent,
        require_auth=False,
    )

    logger.info(f"Agent Card: {base_url}/.well-known/agent-card.json")
    logger.info(f"A2A Endpoint: {base_url}/a2a")
    await server.run_async(host=config.server.host, port=config.server.port)


if __name__ == "__main__":
    asyncio.run(main())

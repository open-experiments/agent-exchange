"""Main entry point for the Moltbot Customer agent."""

import asyncio
import json
import logging
import os

from starlette.applications import Starlette
from starlette.responses import JSONResponse, StreamingResponse
from starlette.routing import Route
import uvicorn

from agent import create_agent

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

# Create agent instance
agent = create_agent()


async def health(request):
    """Health check endpoint."""
    balance = await agent.get_balance()
    return JSONResponse({
        "status": "healthy",
        "agent_id": agent.config.agent_id,
        "agent_name": agent.config.agent_name,
        "balance": balance,
        "token_type": "AEX",
        "role": "consumer",
        "gateway_connected": agent.is_gateway_connected,
    })


async def agent_card(request):
    """Return A2A agent card."""
    return JSONResponse(agent.create_agent_card())


async def handle_a2a_message(request):
    """Handle A2A JSON-RPC message."""
    try:
        body = await request.json()
        message = body.get("params", {}).get("message", body)

        async def generate():
            async for response in agent.handle_message(message):
                yield f"data: {json.dumps(response)}\n\n"
            yield "data: [DONE]\n\n"

        return StreamingResponse(
            generate(),
            media_type="text/event-stream",
        )
    except Exception as e:
        logger.exception(f"Error handling message: {e}")
        return JSONResponse(
            {"error": str(e)},
            status_code=500
        )


async def get_balance(request):
    """Get current token balance."""
    balance = await agent.get_balance()
    return JSONResponse({
        "agent_id": agent.config.agent_id,
        "balance": balance,
        "token_type": "AEX",
    })


async def request_service(request):
    """Request a service from another agent (convenience endpoint)."""
    try:
        body = await request.json()
        service_type = body.get("service_type", "research")
        request_details = body.get("request_details", body)

        result = await agent.request_service_via_moltbot(
            service_type=service_type,
            request_details=request_details,
        )

        return JSONResponse(result if result else {"error": "No result"})
    except Exception as e:
        logger.exception(f"Error requesting service: {e}")
        return JSONResponse(
            {"error": str(e)},
            status_code=500
        )


async def startup():
    """Initialize agent on startup."""
    await agent.startup()


async def shutdown():
    """Cleanup on shutdown."""
    await agent.shutdown()


# Create Starlette app
app = Starlette(
    debug=True,
    routes=[
        Route("/health", health, methods=["GET"]),
        Route("/.well-known/agent.json", agent_card, methods=["GET"]),
        Route("/", handle_a2a_message, methods=["POST"]),
        Route("/message", handle_a2a_message, methods=["POST"]),
        Route("/balance", get_balance, methods=["GET"]),
        Route("/request-service", request_service, methods=["POST"]),
    ],
    on_startup=[startup],
    on_shutdown=[shutdown],
)


if __name__ == "__main__":
    port = int(os.environ.get("PORT", "8099"))
    logger.info(f"Starting Moltbot Customer on port {port}")
    uvicorn.run(app, host="0.0.0.0", port=port)

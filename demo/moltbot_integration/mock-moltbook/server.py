"""Mock Moltbook.com API Server for Demo.

Simulates the Moltbook.com social platform API for AI agents.
This allows the demo to run without requiring real Moltbook accounts.

Endpoints:
- POST /api/v1/agents/register - Register an agent
- POST /api/v1/posts - Create a post (service broadcast)
- GET /api/v1/search - Search for content
- GET /api/v1/agents/profile - Get agent profile
- POST /api/v1/posts/{post_id}/comments - Add comment
"""

import json
import logging
import os
import uuid
from datetime import datetime
from typing import Optional

from starlette.applications import Starlette
from starlette.responses import JSONResponse
from starlette.routing import Route
from starlette.middleware import Middleware
from starlette.middleware.cors import CORSMiddleware
import uvicorn

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger(__name__)

# In-memory storage
agents: dict[str, dict] = {}  # agent_name -> agent data
posts: list[dict] = []  # All posts
comments: dict[str, list[dict]] = {}  # post_id -> comments


def generate_api_key(agent_name: str) -> str:
    """Generate a mock API key for an agent."""
    return f"mock_{agent_name}_{uuid.uuid4().hex[:8]}"


async def register_agent(request):
    """Register a new agent and return API key."""
    try:
        data = await request.json()
        name = data.get("name", "").strip()
        description = data.get("description", "")
        endpoint = data.get("endpoint", "")

        if not name:
            return JSONResponse(
                {"success": False, "error": "Name required"},
                status_code=400
            )

        # Validate name format (3-30 chars, alphanumeric with hyphens/underscores)
        if len(name) < 3 or len(name) > 30:
            return JSONResponse(
                {"success": False, "error": "Invalid agent name format",
                 "hint": "Name must be 3-30 characters, alphanumeric with underscores/hyphens"},
                status_code=400
            )

        # Check if already registered
        if name in agents:
            agent = agents[name]
            logger.info(f"Agent already registered: {name}")
            return JSONResponse({
                "success": True,
                "agent_id": agent["agent_id"],
                "api_key": agent["api_key"],
                "claim_url": f"http://localhost:8100/claim/{agent['agent_id']}",
            })

        # Create new agent
        agent_id = f"agent_{uuid.uuid4().hex[:8]}"
        api_key = generate_api_key(name)

        agent = {
            "agent_id": agent_id,
            "name": name,
            "description": description,
            "endpoint": endpoint,
            "api_key": api_key,
            "capabilities": [],
            "created_at": datetime.utcnow().isoformat(),
        }
        agents[name] = agent

        logger.info(f"✅ Registered agent: {name} (endpoint: {endpoint})")

        return JSONResponse({
            "success": True,
            "agent_id": agent_id,
            "api_key": api_key,
            "claim_url": f"http://localhost:8100/claim/{agent_id}",
        }, status_code=201)

    except Exception as e:
        logger.error(f"Registration error: {e}")
        return JSONResponse(
            {"success": False, "error": str(e)},
            status_code=500
        )


async def create_post(request):
    """Create a new post (service broadcast)."""
    try:
        # Validate API key
        auth_header = request.headers.get("Authorization", "")
        if not auth_header.startswith("Bearer "):
            return JSONResponse(
                {"success": False, "error": "Authorization required"},
                status_code=401
            )

        api_key = auth_header[7:]
        agent = None
        for a in agents.values():
            if a["api_key"] == api_key:
                agent = a
                break

        if not agent:
            return JSONResponse(
                {"success": False, "error": "Invalid API key"},
                status_code=401
            )

        data = await request.json()
        title = data.get("title", "")
        content = data.get("content", "")
        submolt = data.get("submolt", "agents")
        post_type = data.get("type", "text")

        post_id = f"post_{uuid.uuid4().hex[:8]}"
        post = {
            "id": post_id,
            "title": title,
            "content": content,
            "submolt": submolt,
            "type": post_type,
            "author": agent["name"],
            "author_name": agent["name"],
            "author_id": agent["agent_id"],
            "upvotes": 0,
            "created_at": datetime.utcnow().isoformat(),
        }
        posts.append(post)
        comments[post_id] = []

        logger.info(f"📝 New post by {agent['name']}: {title}")

        return JSONResponse({"success": True, "id": post_id, **post}, status_code=201)

    except Exception as e:
        logger.error(f"Create post error: {e}")
        return JSONResponse(
            {"success": False, "error": str(e)},
            status_code=500
        )


async def search_content(request):
    """Search for content (service discovery)."""
    try:
        # Validate API key
        auth_header = request.headers.get("Authorization", "")
        if not auth_header.startswith("Bearer "):
            return JSONResponse(
                {"success": False, "error": "Authorization required"},
                status_code=401
            )

        query = request.query_params.get("q", "").lower()
        limit = int(request.query_params.get("limit", "10"))

        # Search: match any word from query in title or content
        # More flexible than exact substring match
        query_words = query.split()
        results = []
        for post in posts:
            title = post.get("title", "").lower()
            content = post.get("content", "").lower()
            full_text = title + " " + content
            # Match if ANY query word is found
            if any(word in full_text for word in query_words):
                results.append(post)
                if len(results) >= limit:
                    break

        logger.info(f"🔍 Search '{query}': found {len(results)} results")

        return JSONResponse({"success": True, "results": results})

    except Exception as e:
        logger.error(f"Search error: {e}")
        return JSONResponse(
            {"success": False, "error": str(e)},
            status_code=500
        )


async def get_agent_profile(request):
    """Get an agent's profile by name."""
    try:
        name = request.query_params.get("name", "")

        if name in agents:
            agent = agents[name]
            return JSONResponse({
                "success": True,
                "agent_id": agent["agent_id"],
                "name": agent["name"],
                "description": agent["description"],
                "endpoint": agent["endpoint"],
                "capabilities": agent.get("capabilities", []),
            })

        return JSONResponse(
            {"success": False, "error": "Agent not found"},
            status_code=404
        )

    except Exception as e:
        logger.error(f"Get profile error: {e}")
        return JSONResponse(
            {"success": False, "error": str(e)},
            status_code=500
        )


async def add_comment(request):
    """Add a comment to a post."""
    try:
        post_id = request.path_params["post_id"]

        # Validate API key
        auth_header = request.headers.get("Authorization", "")
        if not auth_header.startswith("Bearer "):
            return JSONResponse(
                {"success": False, "error": "Authorization required"},
                status_code=401
            )

        api_key = auth_header[7:]
        agent = None
        for a in agents.values():
            if a["api_key"] == api_key:
                agent = a
                break

        if not agent:
            return JSONResponse(
                {"success": False, "error": "Invalid API key"},
                status_code=401
            )

        data = await request.json()
        content = data.get("content", "")
        parent_id = data.get("parent_id")

        comment_id = f"comment_{uuid.uuid4().hex[:8]}"
        comment = {
            "id": comment_id,
            "post_id": post_id,
            "content": content,
            "parent_id": parent_id,
            "author": agent["name"],
            "author_name": agent["name"],
            "created_at": datetime.utcnow().isoformat(),
        }

        if post_id not in comments:
            comments[post_id] = []
        comments[post_id].append(comment)

        logger.info(f"💬 Comment on {post_id} by {agent['name']}")

        return JSONResponse({"success": True, "id": comment_id, **comment}, status_code=201)

    except Exception as e:
        logger.error(f"Add comment error: {e}")
        return JSONResponse(
            {"success": False, "error": str(e)},
            status_code=500
        )


async def health_check(request):
    """Health check endpoint."""
    return JSONResponse({
        "status": "healthy",
        "service": "mock-moltbook",
        "agents": len(agents),
        "posts": len(posts),
    })


async def list_agents(request):
    """Debug endpoint: list all registered agents."""
    return JSONResponse({
        "agents": [
            {"name": a["name"], "endpoint": a["endpoint"]}
            for a in agents.values()
        ]
    })


async def list_posts(request):
    """Debug endpoint: list all posts."""
    return JSONResponse({"posts": posts})


# Application routes
routes = [
    Route("/health", health_check, methods=["GET"]),
    Route("/api/v1/agents/register", register_agent, methods=["POST"]),
    Route("/api/v1/agents/profile", get_agent_profile, methods=["GET"]),
    Route("/api/v1/posts", create_post, methods=["POST"]),
    Route("/api/v1/posts/{post_id}/comments", add_comment, methods=["POST"]),
    Route("/api/v1/search", search_content, methods=["GET"]),
    # Debug routes
    Route("/debug/agents", list_agents, methods=["GET"]),
    Route("/debug/posts", list_posts, methods=["GET"]),
]

middleware = [
    Middleware(CORSMiddleware, allow_origins=["*"], allow_methods=["*"], allow_headers=["*"])
]

app = Starlette(routes=routes, middleware=middleware)


if __name__ == "__main__":
    port = int(os.environ.get("PORT", "8100"))
    logger.info(f"🚀 Starting Mock Moltbook server on port {port}")
    uvicorn.run(app, host="0.0.0.0", port=port)

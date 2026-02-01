# Dockerfile for Moltbot Demo Agents
FROM python:3.12-slim

ARG AGENT_DIR

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Copy common module first
COPY common /app/common

# Copy agent-specific files
COPY ${AGENT_DIR} /app/agent

# Install dependencies
RUN pip install --no-cache-dir -r /app/common/requirements.txt
RUN if [ -f /app/agent/requirements.txt ]; then pip install --no-cache-dir -r /app/agent/requirements.txt; fi

# Set working directory to agent
WORKDIR /app/agent

# Create non-root user
RUN useradd -m -u 1000 appuser && chown -R appuser:appuser /app
USER appuser

# Default command
CMD ["python", "main.py"]

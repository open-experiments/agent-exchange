FROM python:3.11-slim

ARG AGENT_DIR

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Copy common utilities first
COPY common/ /app/common/

# Install common dependencies first
RUN pip install --no-cache-dir -r /app/common/requirements.txt

# Copy agent-specific files
COPY ${AGENT_DIR}/ /app/

# Install agent-specific dependencies (skip the -r ../common/requirements.txt line)
RUN grep -v "^-r " requirements.txt > agent_requirements.txt || true && \
    pip install --no-cache-dir -r agent_requirements.txt || true

# Run the agent
CMD ["python", "main.py"]

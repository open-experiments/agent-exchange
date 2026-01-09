FROM python:3.11-slim

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements and install
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy application
COPY main.py .

# Expose port (Cloud Run provides PORT env var, default 8080)
EXPOSE 8080

# Run the UI with gunicorn, respecting PORT env var
# Increase timeout to 180s for long-running A2A calls to legal agents
CMD gunicorn --bind 0.0.0.0:${PORT:-8080} --timeout 180 --workers 2 main:me

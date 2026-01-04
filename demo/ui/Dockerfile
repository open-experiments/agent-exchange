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

# Expose port
EXPOSE 8501

# Run the UI with gunicorn (mesop binds to localhost by default, gunicorn allows 0.0.0.0)
CMD ["gunicorn", "--bind", "0.0.0.0:8501", "main:me"]

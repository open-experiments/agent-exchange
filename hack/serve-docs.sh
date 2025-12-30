#!/bin/bash

# Serve API documentation locally
# Usage: ./hack/serve-docs.sh [port]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PORT="${1:-8090}"

echo "Serving API documentation at http://localhost:$PORT"
echo "Press Ctrl+C to stop"
echo ""

cd "$PROJECT_ROOT/api"

# Use Python's built-in HTTP server
if command -v python3 &> /dev/null; then
    python3 -m http.server "$PORT"
elif command -v python &> /dev/null; then
    python -m SimpleHTTPServer "$PORT"
else
    echo "Error: Python is required to serve documentation"
    exit 1
fi


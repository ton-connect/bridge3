#!/bin/bash

# Script to run integration tests with bridge-sdk
set -e

BRIDGE_PORT=${PORT:-8081}
BRIDGE_URL=${BRIDGE_URL:-http://localhost:$BRIDGE_PORT}
BRIDGE_BINARY="./callmebridge"

echo "🚀 Starting integration tests with bridge-sdk..."
echo "Bridge URL: $BRIDGE_URL"

# Build the bridge if it doesn't exist
if [ ! -f "$BRIDGE_BINARY" ]; then
    echo "🔨 Building bridge..."
    make build
fi

# Start the bridge in background
echo "🌉 Starting bridge server on port $BRIDGE_PORT..."
$BRIDGE_BINARY &
BRIDGE_PID=$!

# Function to cleanup
cleanup() {
    echo "🧹 Cleaning up..."
    if kill -0 $BRIDGE_PID 2>/dev/null; then
        echo "🛑 Stopping bridge server (PID: $BRIDGE_PID)..."
        kill $BRIDGE_PID
        wait $BRIDGE_PID 2>/dev/null || true
    fi
}

# Set trap to cleanup on exit
trap cleanup EXIT

# Wait for bridge to be ready
echo "⏳ Waiting for bridge to be ready..."
for i in {1..30}; do
    if curl -f "$BRIDGE_URL/health" &> /dev/null; then
        echo "✅ Bridge is ready!"
        break
    fi
    if [ $i -eq 30 ]; then
        echo "❌ Bridge failed to start within 30 seconds"
        exit 1
    fi
        echo "⏳ Waiting for bridge to be ready... ($i/30)"
    sleep 1
done

# Run bridge-sdk tests
echo "🧪 Running bridge-sdk tests against live bridge..."
BRIDGE_URL="$BRIDGE_URL/bridge" ./scripts/test-bridge-sdk.sh

echo "🎉 Integration tests completed successfully!"
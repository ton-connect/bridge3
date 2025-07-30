#!/bin/bash

# Script to run integration tests with bridge-sdk
set -e

STORAGE=${STORAGE:-memory}
BRIDGE_PORT=${PORT:-8081}
BRIDGE_URL=${BRIDGE_URL:-http://localhost:$BRIDGE_PORT}
BRIDGE_BINARY="./callmebridge"

echo "🚀 Starting integration tests with bridge-sdk..."
echo "Storage backend: $STORAGE"
echo "Bridge URL: $BRIDGE_URL"

# Set up storage-specific environment and dependencies
case $STORAGE in
    postgres)
        echo "🐘 Setting up PostgreSQL..."
        # Start PostgreSQL in Docker if not running
        if ! docker ps --format 'table {{.Names}}' | grep -q "postgres"; then
            echo "🐳 Starting PostgreSQL container..."
            docker run -d --name postgres-test \
                -e POSTGRES_DB=bridge \
                -e POSTGRES_USER=bridge_user \
                -e POSTGRES_PASSWORD=bridge_password \
                -p 5432:5432 \
                postgres:15-alpine
            
            # Wait for PostgreSQL to be ready
            echo "⏳ Waiting for PostgreSQL to be ready..."
            timeout 30 bash -c 'until docker exec postgres-test pg_isready -U bridge_user -d bridge; do sleep 1; done'
        fi
        export POSTGRES_URI="postgres://bridge_user:bridge_password@localhost:5432/bridge?sslmode=disable"
        ;;
    valkey)
        echo "🔄 Setting up Valkey..."
        # Start Valkey in Docker if not running
        if ! docker ps --format 'table {{.Names}}' | grep -q "valkey"; then
            echo "🐳 Starting Valkey container..."
            docker run -d --name valkey-test \
                -p 6379:6379 \
                valkey/valkey:7.2-alpine
            
            # Wait for Valkey to be ready
            echo "⏳ Waiting for Valkey to be ready..."
            timeout 30 bash -c 'until docker exec valkey-test valkey-cli ping | grep -q PONG; do sleep 1; done'
        fi
        export VALKEY_URI="valkey://localhost:6379"
        ;;
    memory)
        echo "🧠 Using in-memory storage..."
        # No setup needed for memory storage
        ;;
    *)
        echo "❌ Unknown storage type: $STORAGE"
        echo "Supported types: postgres, valkey, memory"
        exit 1
        ;;
esac

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
    
    # Clean up Docker containers if we started them
    case $STORAGE in
        postgres)
            if docker ps --format 'table {{.Names}}' | grep -q "postgres-test"; then
                echo "🐳 Stopping PostgreSQL test container..."
                docker stop postgres-test >/dev/null 2>&1 || true
                docker rm postgres-test >/dev/null 2>&1 || true
            fi
            ;;
        valkey)
            if docker ps --format 'table {{.Names}}' | grep -q "valkey-test"; then
                echo "🐳 Stopping Valkey test container..."
                docker stop valkey-test >/dev/null 2>&1 || true
                docker rm valkey-test >/dev/null 2>&1 || true
            fi
            ;;
    esac
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

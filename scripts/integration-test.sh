#!/bin/bash

# Enhanced integration test script supporting multiple storage types
# Usage: ./integration-test.sh [storage_type]
# storage_type: memory (default), postgres, kafka

set -e

# Configuration
STORAGE_TYPE=${1:-memory}
BRIDGE_PORT=${PORT:-8082}  # Use different port to avoid conflicts
BRIDGE_URL=${BRIDGE_URL:-http://localhost:$BRIDGE_PORT}
BRIDGE_BINARY="./callmebridge"

# Storage-specific configuration
POSTGRES_CONTAINER=""
KAFKA_CONTAINER=""
TEST_TIMEOUT=60

echo "🚀 Starting integration tests with $STORAGE_TYPE storage..."
echo "Bridge URL: $BRIDGE_URL"

# Function to cleanup
cleanup() {
    echo "🧹 Cleaning up..."
    
    # Stop bridge if running
    if [ ! -z "$BRIDGE_PID" ] && kill -0 $BRIDGE_PID 2>/dev/null; then
        echo "� Stopping bridge server (PID: $BRIDGE_PID)..."
        kill $BRIDGE_PID
        wait $BRIDGE_PID 2>/dev/null || true
    fi
    
    # Clean up storage-specific resources
    case $STORAGE_TYPE in
        postgres)
            if [ ! -z "$POSTGRES_CONTAINER" ]; then
                echo "🗄️ Stopping PostgreSQL container..."
                docker stop $POSTGRES_CONTAINER >/dev/null 2>&1 || true
                docker rm $POSTGRES_CONTAINER >/dev/null 2>&1 || true
            fi
            ;;
        kafka)
            if [ ! -z "$KAFKA_CONTAINER" ]; then
                echo "📨 Stopping Kafka container..."
                docker stop $KAFKA_CONTAINER >/dev/null 2>&1 || true
                docker rm $KAFKA_CONTAINER >/dev/null 2>&1 || true
            fi
            ;;
    esac
}

# Set trap to cleanup on exit
trap cleanup EXIT

# Function to setup storage
setup_storage() {
    case $STORAGE_TYPE in
        memory)
            echo "💾 Using in-memory storage (no setup required)"
            export STORAGE_TYPE="memory"
            ;;
        postgres)
            echo "🗄️ Setting up PostgreSQL storage..."
            
            # Start PostgreSQL container
            POSTGRES_CONTAINER=$(docker run -d \
                --name "integration-postgres-$$" \
                -e POSTGRES_DB=bridge \
                -e POSTGRES_USER=bridge_user \
                -e POSTGRES_PASSWORD=bridge_password \
                -p 5433:5432 \
                postgres:15-alpine)
            
            echo "⏳ Waiting for PostgreSQL to be ready..."
            for i in {1..30}; do
                if docker exec $POSTGRES_CONTAINER pg_isready -U bridge_user -d bridge >/dev/null 2>&1; then
                    echo "✅ PostgreSQL is ready!"
                    break
                fi
                if [ $i -eq 30 ]; then
                    echo "❌ PostgreSQL failed to start within 30 seconds"
                    exit 1
                fi
                sleep 1
            done
            
            export STORAGE_TYPE="postgres"
            export POSTGRES_URI="postgres://bridge_user:bridge_password@localhost:5433/bridge?sslmode=disable"
            ;;
        kafka)
            echo "📨 Setting up Kafka storage..."
            
            # Start Kafka container in KRaft mode
            KAFKA_CONTAINER=$(docker run -d \
                --name "integration-kafka-$$" \
                -p 9094:9092 \
                -e KAFKA_NODE_ID=1 \
                -e KAFKA_LISTENER_SECURITY_PROTOCOL_MAP='CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT,PLAINTEXT_HOST:PLAINTEXT' \
                -e KAFKA_ADVERTISED_LISTENERS='PLAINTEXT://localhost:29092,PLAINTEXT_HOST://localhost:9094' \
                -e KAFKA_LISTENERS='PLAINTEXT://localhost:29092,CONTROLLER://localhost:29093,PLAINTEXT_HOST://0.0.0.0:9092' \
                -e KAFKA_INTER_BROKER_LISTENER_NAME='PLAINTEXT' \
                -e KAFKA_CONTROLLER_LISTENER_NAMES='CONTROLLER' \
                -e KAFKA_CONTROLLER_QUORUM_VOTERS='1@localhost:29093' \
                -e KAFKA_PROCESS_ROLES='broker,controller' \
                -e KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=1 \
                -e KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR=1 \
                -e KAFKA_TRANSACTION_STATE_LOG_MIN_ISR=1 \
                -e KAFKA_DEFAULT_REPLICATION_FACTOR=1 \
                -e KAFKA_AUTO_CREATE_TOPICS_ENABLE='true' \
                -e KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS=0 \
                -e CLUSTER_ID='MkU3OEVBNTcwNTJENDM2Qk' \
                confluentinc/cp-kafka:7.4.0)
            
            echo "⏳ Waiting for Kafka to be ready..."
            for i in {1..60}; do
                if docker exec $KAFKA_CONTAINER kafka-broker-api-versions --bootstrap-server localhost:9092 >/dev/null 2>&1; then
                    echo "✅ Kafka is ready!"
                    break
                fi
                if [ $i -eq 60 ]; then
                    echo "❌ Kafka failed to start within 60 seconds"
                    exit 1
                fi
                sleep 1
            done
            
            export STORAGE_TYPE="kafka"
            export KAFKA_BROKERS="localhost:9094"
            export KAFKA_TOPIC="bridge-integration-test"
            export KAFKA_CONSUMER_GROUP="bridge-integration-consumer"
            ;;
        *)
            echo "❌ Unknown storage type: $STORAGE_TYPE"
            echo "Supported types: memory, postgres, kafka"
            exit 1
            ;;
    esac
}

# Build the bridge if it doesn't exist
if [ ! -f "$BRIDGE_BINARY" ]; then
    echo "🔨 Building bridge..."
    make build
fi

# Setup storage
setup_storage

# Start the bridge in background
echo "🌉 Starting bridge server on port $BRIDGE_PORT with $STORAGE_TYPE storage..."
export PORT=$BRIDGE_PORT
export CORS_ENABLE=true
export HEARTBEAT_INTERVAL=5

$BRIDGE_BINARY &
BRIDGE_PID=$!

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

# Check readiness (including storage health)
echo "🏥 Checking bridge readiness (including storage health)..."
if ! curl -f "$BRIDGE_URL/ready" &> /dev/null; then
    echo "❌ Bridge readiness check failed"
    exit 1
fi
echo "✅ Bridge readiness check passed!"

# Run bridge-sdk tests
echo "🧪 Running bridge-sdk tests against live bridge with $STORAGE_TYPE storage..."
# Use timeout if available, otherwise run without it
if command -v timeout >/dev/null 2>&1; then
    BRIDGE_URL="$BRIDGE_URL/bridge" timeout $TEST_TIMEOUT ./scripts/test-bridge-sdk.sh
else
    BRIDGE_URL="$BRIDGE_URL/bridge" ./scripts/test-bridge-sdk.sh
fi

# Run storage-specific tests
echo "🔍 Running storage-specific integration tests..."
case $STORAGE_TYPE in
    kafka)
        echo "📨 Testing Kafka-specific functionality..."
        
        # Test multiple consumers
        CLIENT_A="integration-client-a"
        CLIENT_B="integration-client-b"
        
        # Start SSE connections
        curl -N -H "Accept: text/event-stream" "$BRIDGE_URL/events?client_id=$CLIENT_A" > /tmp/client_a_output.txt &
        SSE_PID_A=$!
        
        curl -N -H "Accept: text/event-stream" "$BRIDGE_URL/events?client_id=$CLIENT_B" > /tmp/client_b_output.txt &
        SSE_PID_B=$!
        
        sleep 2  # Allow connections to establish
        
        # Send messages
        curl -X POST "$BRIDGE_URL/message?client_id=sender&to=$CLIENT_A&ttl=60" -d "Test message A" >/dev/null 2>&1
        curl -X POST "$BRIDGE_URL/message?client_id=sender&to=$CLIENT_B&ttl=60" -d "Test message B" >/dev/null 2>&1
        
        sleep 3  # Allow message processing
        
        # Check if messages were received
        kill $SSE_PID_A $SSE_PID_B 2>/dev/null || true
        wait $SSE_PID_A $SSE_PID_B 2>/dev/null || true
        
        if grep -q "Test message A" /tmp/client_a_output.txt && grep -q "Test message B" /tmp/client_b_output.txt; then
            echo "✅ Kafka multi-client test passed!"
        else
            echo "❌ Kafka multi-client test failed!"
            echo "Client A output:"
            cat /tmp/client_a_output.txt 2>/dev/null || echo "No output"
            echo "Client B output:"
            cat /tmp/client_b_output.txt 2>/dev/null || echo "No output"
            exit 1
        fi
        
        rm -f /tmp/client_a_output.txt /tmp/client_b_output.txt
        ;;
    postgres)
        echo "🗄️ Testing PostgreSQL-specific functionality..."
        # Could add database-specific tests here
        ;;
    memory)
        echo "💾 Testing memory storage functionality..."
        # Memory storage tests (if any specific tests needed)
        ;;
esac

echo "🎉 Integration tests for $STORAGE_TYPE storage completed successfully!"

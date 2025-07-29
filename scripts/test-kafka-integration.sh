#!/bin/bash

# Integration test for CallMeBridge with Kafka storage

set -e

BRIDGE_URL="http://localhost:8081"
CLIENT_ID="test-client-$(date +%s)"
MESSAGE_CONTENT="Hello from Kafka integration test!"

echo "🧪 CallMeBridge Kafka Integration Test"
echo "======================================"
echo ""

# Function to cleanup background processes
cleanup() {
    echo "🧹 Cleaning up..."
    if [ ! -z "$BRIDGE_PID" ]; then
        kill $BRIDGE_PID 2>/dev/null || true
    fi
    if [ ! -z "$SSE_PID" ]; then
        kill $SSE_PID 2>/dev/null || true
    fi
}

trap cleanup EXIT

# Check if Kafka is running
echo "📋 Checking Kafka availability..."
if ! nc -z localhost 9092; then
    echo "❌ Kafka is not running on localhost:9092"
    echo "Please start Kafka with: docker-compose -f docker-compose.kafka.yml up -d kafka"
    exit 1
fi
echo "✅ Kafka is available"

# Check if bridge is built
echo "🔨 Building CallMeBridge..."
make build

# Start bridge with Kafka storage
echo "🌉 Starting CallMeBridge with Kafka storage..."
export STORAGE_TYPE="kafka"
export KAFKA_BROKERS="localhost:9092"
export KAFKA_TOPIC="bridge-test-messages"
export KAFKA_CONSUMER_GROUP="bridge-test-consumer"
export PORT="8081"
export CORS_ENABLE="true"

./callmebridge > bridge.log 2>&1 &
BRIDGE_PID=$!

# Wait for bridge to start
echo "⏳ Waiting for bridge to start..."
for i in {1..30}; do
    if curl -s "$BRIDGE_URL/health" > /dev/null 2>&1; then
        break
    fi
    sleep 1
    if [ $i -eq 30 ]; then
        echo "❌ Bridge failed to start within 30 seconds"
        echo "Bridge logs:"
        cat bridge.log
        exit 1
    fi
done
echo "✅ Bridge is running"

# Test health endpoint
echo "🏥 Testing health endpoint..."
HEALTH_RESPONSE=$(curl -s "$BRIDGE_URL/health")
if echo "$HEALTH_RESPONSE" | grep -q '"status":"ok"'; then
    echo "✅ Health check passed"
else
    echo "❌ Health check failed: $HEALTH_RESPONSE"
    exit 1
fi

# Test ready endpoint
echo "🚦 Testing ready endpoint..."
READY_RESPONSE=$(curl -s "$BRIDGE_URL/ready")
if echo "$READY_RESPONSE" | grep -q '"status":"ready"'; then
    echo "✅ Ready check passed"
else
    echo "❌ Ready check failed: $READY_RESPONSE"
    exit 1
fi

# Start SSE connection in background
echo "📡 Starting SSE connection..."
curl -s -N "$BRIDGE_URL/bridge/events?client_id=$CLIENT_ID" > sse_output.txt &
SSE_PID=$!

# Wait a moment for SSE connection to establish
sleep 2

# Send a test message
echo "📤 Sending test message..."
SEND_RESPONSE=$(curl -s -X POST "$BRIDGE_URL/bridge/message" \
    -H "Content-Type: application/json" \
    -d "{\"from\":\"$CLIENT_ID\",\"message\":\"$MESSAGE_CONTENT\"}")

if echo "$SEND_RESPONSE" | grep -q '"status":"ok"'; then
    echo "✅ Message sent successfully"
else
    echo "❌ Message send failed: $SEND_RESPONSE"
    exit 1
fi

# Wait for message to be processed and delivered
echo "⏳ Waiting for message delivery..."
sleep 5

# Check if message was received via SSE
echo "📥 Checking received messages..."
if [ -f sse_output.txt ] && grep -q "$MESSAGE_CONTENT" sse_output.txt; then
    echo "✅ Message received via SSE"
    echo "📋 Received content:"
    grep "$MESSAGE_CONTENT" sse_output.txt | head -1
else
    echo "❌ Message not received via SSE"
    echo "SSE output:"
    cat sse_output.txt 2>/dev/null || echo "No SSE output file"
    exit 1
fi

# Test metrics endpoint
echo "📊 Testing metrics endpoint..."
METRICS_RESPONSE=$(curl -s "$BRIDGE_URL/metrics")
if echo "$METRICS_RESPONSE" | grep -q "bridge_health_status"; then
    echo "✅ Metrics endpoint working"
else
    echo "❌ Metrics endpoint failed"
    exit 1
fi

echo ""
echo "🎉 All tests passed!"
echo "✅ Kafka storage integration is working correctly"
echo ""

# Show some metrics
echo "📈 Bridge metrics:"
echo "$(curl -s "$BRIDGE_URL/metrics" | grep -E "(bridge_health_status|bridge_ready_status)" | head -2)"

# Clean up test files
rm -f sse_output.txt bridge.log

echo ""
echo "🧪 Integration test completed successfully!"

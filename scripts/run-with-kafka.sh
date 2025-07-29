#!/bin/bash

# Example script to run CallMeBridge with Kafka storage

echo "🚀 Starting CallMeBridge with Kafka storage"
echo ""

# Check if Kafka is running
echo "📋 Checking Kafka availability..."
if ! nc -z localhost 9092; then
    echo "❌ Kafka is not running on localhost:9092"
    echo "💡 Please start Kafka first:"
    echo "   Option 1: Use docker-compose (KRaft mode - no Zookeeper needed!):"
    echo "     docker-compose -f docker-compose.kafka.yml up -d kafka"
    echo ""
    echo "   Option 2: Use local Kafka installation with KRaft:"
    echo "     # Generate cluster ID"
    echo "     KAFKA_CLUSTER_ID=\$(kafka-storage.sh random-uuid)"
    echo "     # Format log directories"
    echo "     kafka-storage.sh format -t \$KAFKA_CLUSTER_ID -c /usr/local/etc/kafka/kraft/server.properties"
    echo "     # Start Kafka in KRaft mode"
    echo "     kafka-server-start.sh /usr/local/etc/kafka/kraft/server.properties"
    echo ""
    exit 1
fi

echo "✅ Kafka is running"
echo ""

# Set environment variables for Kafka storage
export STORAGE_TYPE="kafka"
export KAFKA_BROKERS="localhost:9092"
export KAFKA_TOPIC="bridge-messages"
export KAFKA_CONSUMER_GROUP="bridge-consumer"
export PORT="8081"
export CORS_ENABLE="true"
export HEARTBEAT_INTERVAL="10"
export RPS_LIMIT="1000"
export CONNECTIONS_LIMIT="200"

echo "🔧 Configuration:"
echo "   Storage Type: $STORAGE_TYPE"
echo "   Kafka Brokers: $KAFKA_BROKERS"
echo "   Kafka Topic: $KAFKA_TOPIC"
echo "   Consumer Group: $KAFKA_CONSUMER_GROUP"
echo "   Port: $PORT"
echo ""

# Build and run the bridge
echo "🔨 Building CallMeBridge..."
if ! make build; then
    echo "❌ Failed to build CallMeBridge"
    exit 1
fi

echo "✅ Build successful"
echo ""

echo "🌉 Starting CallMeBridge with Kafka storage..."
echo "   API endpoints will be available at:"
echo "   - http://localhost:8081/bridge/events (SSE)"
echo "   - http://localhost:8081/bridge/message (POST)"
echo "   - http://localhost:8081/health (Health check)"
echo "   - http://localhost:8081/ready (Readiness check)"
echo "   - http://localhost:8081/metrics (Prometheus metrics)"
echo ""

# Run the bridge
./callmebridge

# bridge

[HTTP bridge](https://github.com/ton-connect/docs/blob/main/bridge.md) implementation for TON Connect 2.0.

## 🚀Quick Start

```bash
git clone https://github.com/callmedenchick/callmebridge
cd callmebridge
make build
./callmebridge
```

### PostgreSQL Storage
```bash
# Start PostgreSQL and set environment
export STORAGE_TYPE="postgres"
export POSTGRES_URI="postgres://user:pass@localhost/bridge"
./callmebridge
```

### Kafka Storage
```bash
# Start Kafka (using Docker Compose)
docker-compose -f docker-compose.kafka.yml up -d kafka

# Or use the provided script
./scripts/run-with-kafka.sh
```

### Full Multi-Instance Setup with Kafka
```bash
# Start the entire stack with load balancer
docker-compose -f docker-compose.kafka.yml up
# Access via http://localhost:8080 (nginx load balancer)
```

## 📋Requirements

- Go 1.23+
- PostgreSQL (optional)
- Node.js & npm (for testing)

## ⚙️Configuration

Configure using environment variables:

```bash
PORT=8081                    # Server port

# Storage Configuration
STORAGE_TYPE="memory"        # Storage type: memory, postgres, kafka
POSTGRES_URI="postgres://user:pass@host/dbname"  # PostgreSQL connection (for postgres storage)

# Kafka Configuration (for kafka storage)
KAFKA_BROKERS="localhost:9092,localhost:9093"  # Comma-separated Kafka broker addresses
KAFKA_TOPIC="bridge-messages"                  # Kafka topic name (default: bridge-messages)
KAFKA_CONSUMER_GROUP="bridge-consumer"         # Kafka consumer group (default: bridge-consumer)

# Other Configuration
WEBHOOK_URL=""               # Webhook URL for message forwarding
COPY_TO_URL=""              # URL to copy messages to
CORS_ENABLE=false           # Enable CORS
HEARTBEAT_INTERVAL=10       # Heartbeat interval in seconds
RPS_LIMIT=1000              # Rate limit (requests per second)
CONNECTIONS_LIMIT=200       # Maximum concurrent connections
SELF_SIGNED_TLS=false       # Use self-signed TLS certificate
```

### Storage Types

**Memory Storage** (default)
- Fast, in-memory storage
- Data is lost on restart
- Best for development and testing

**PostgreSQL Storage**
- Persistent storage with automatic migrations
- Supports high availability setups
- Automatic cleanup of expired messages

### Kafka Storage

**Kafka Storage** uses modern **KRaft mode** (Kafka Raft metadata mode) instead of Zookeeper:

✅ **KRaft Benefits:**
- **Simpler architecture**: No separate Zookeeper cluster needed
- **Faster startup**: Reduced coordination overhead
- **Better performance**: Lower latency for metadata operations
- **Easier scaling**: Single service to manage and scale
- **Production-ready**: Available since Kafka 3.3.0+

**Features:**
- Distributed, scalable message storage
- Hybrid approach: messages stored in Kafka with in-memory caching
- Automatic message expiration and cleanup
- Supports multiple bridge instances
- Built-in health checks and monitoring

## 🛠️API Endpoints

### Bridge Endpoints

- `GET /bridge/events` - Server-Sent Events for real-time message delivery
- `POST /bridge/message` - Send messages through the bridge

### Health & Monitoring

- `GET /health` - Health check endpoint
- `GET /ready` - Readiness check (includes database connectivity)
- `GET /metrics` - Prometheus metrics

## 📊Monitoring

CallMeBridge provides comprehensive monitoring capabilities:

### Prometheus Metrics

- `number_of_active_connections` - Active WebSocket connections
- `number_of_active_subscriptions` - Active client subscriptions
- `number_of_transfered_messages` - Total messages transferred
- `number_of_delivered_messages` - Total messages delivered
- `number_of_bad_requests` - Bad request count
- `number_of_client_ids_per_connection` - Client IDs per connection histogram
- `bridge_token_usage` - Token usage by bypass tokens
- `bridge_health_status` - Health status of the bridge (1 = healthy, 0 = unhealthy)
- `bridge_ready_status` - Ready status of the bridge (1 = ready, 0 = not ready)

Made with ❤️ for the TON ecosystem

# bridge

[HTTP bridge](https://github.com/ton-connect/docs/blob/main/bridge.md) implementation for TON Connect 2.0.

## 🚀Quick Start

```bash
git clone https://github.com/callmedenchick/callmebridge
cd callmebridge
make build
./callmebridge
```

## 📋Requirements

- Go 1.23+
- PostgreSQL (optional)
- NATS JetStream (optional)
- Node.js & npm (for testing)

## ⚙️Configuration

Configure using environment variables:

```bash
PORT=8081                        # Server port
POSTGRES_URI="postgres://user:pass@host/dbname"  # PostgreSQL database connection
NATS_URI="nats://localhost:4222" # NATS JetStream connection
```

## 🗄️Storage Backends

CallMeBridge supports multiple storage backends:

### In-Memory Storage
- **Use case**: Development, testing
- **Configuration**: Used by defaul when neither POSTGRES_URI nor NATS_URI is set)
- **Pros**: No need to setup
- **Cons**: Not suitable for production due to message loss after restart

### PostgreSQL
- **Use case**: Production deployments with persistence requirements
- **Configuration**: Set `POSTGRES_URI` to a PostgreSQL connection string
- **Pros**: ACID compliance, mature, well-tested
- **Cons**: Requires PostgreSQL server

### NATS JetStream
- **Use case**: High-throughput, distributed deployments
- **Configuration**: Set `NATS_URI` to a NATS server URL (e.g., `nats://localhost:4222`)
- **Pros**: High performance, distributed, built-in clustering
- **Cons**: Requires NATS JetStream server

**Storage Selection Priority:**
1. `NATS_URI` - If set, NATS JetStream will be used
2. `POSTGRES_URI` - If NATS_URI is not set, checks for PostgreSQL or NATS URL format
3. In-Memory - If neither is set, uses in-memory storage

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

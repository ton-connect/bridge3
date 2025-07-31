# bridge

[HTTP bridge](https://github.com/ton-connect/docs/blob/main/bridge.md) implementation for TON Connect 2.0.


> ⚠️ This bridge is currently under development. **Do not use in production** until the first stable release.


## 🚀Quick Start

```bash
git clone https://github.com/ton-connect/bridge3
cd bridge
make build
./bridge
```

Use `make help` to see all available commands and storage options.

## 📋Requirements

- Go 1.23+
- PostgreSQL or Valkey/Redis (optional, depending on storage backend)
- Node.js & npm (for testing)

## ⚙️Configuration

Configure using environment variables:

```bash
PORT=8081                       # Server port
VALKEY_URI="valkey://host:6379" # Valkey connection string
POSTGRES_URI="postgres://user:pass@host/dbname"  # PostgreSQL connection
CORS_ENABLE=true                # Enable CORS headers
HEARTBEAT_INTERVAL=10           # Heartbeat interval in seconds
RPS_LIMIT=1000                  # Rate limit per second
CONNECTIONS_LIMIT=200           # Maximum concurrent connections
```

## 💾Storage

- **Valkey**: Redis-compatible storage for high performance
- **PostgreSQL**: Relational database with full persistence
- **Memory**: In-memory storage (no persistence, fastest for testing)

**Storage Selection Logic:**
- If `POSTGRES_URI` is set → PostgreSQL storage
- If `VALKEY_URI` is set → Valkey storage  
- If both are set → Valkey takes precedence
- If neither is set → Memory storage

## 🛠️API Endpoints

### Bridge Endpoints

- `GET /bridge/events` - Server-Sent Events for real-time message delivery
- `POST /bridge/message` - Send messages through the bridge

### Health & Monitoring

- `GET /health` - Health check endpoint
- `GET /ready` - Readiness check (includes database connectivity)
- `GET /metrics` - Prometheus metrics

## 📊Monitoring

Bridge provides comprehensive monitoring capabilities:

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

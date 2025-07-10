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
- Node.js & npm (for testing)

## ⚙️Configuration

Configure using environment variables:

```bash
PORT=8081                    # Server port
POSTGRES_URI="postgres://user:pass@host/dbname"  # Database connection
```

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

Made with ❤️ for the TON ecosystem

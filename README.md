# bridge
[http bridge](https://github.com/ton-connect/docs/blob/main/bridge.md) for tonconnect 2.0:

## requirements
- Golang 1.23
- postgres
- Node.js and npm (for bridge-sdk testing)

## how to install
- git clone https://github.com/ton-connect/bridge
- cd bridge
- go build ./ 
- go run bridge

## testing

### Unit Tests
Run Go unit tests:
```bash
make test
```

### Bridge SDK Tests
Test against the official ton-connect bridge-sdk:
```bash
# Test against a running bridge instance
make test-bridge-sdk

# Run full integration tests (starts bridge, runs tests, stops bridge)
make integration-test

# Run all tests (unit + bridge-sdk)
make test-all
```

### Bridge SDK Test Requirements
- Node.js and npm must be installed
- The bridge-sdk tests will clone the official repository automatically
- Set `BRIDGE_URL` environment variable to test against a different bridge instance
- Default bridge URL is `http://localhost:8081`

## environments
PORT

POSTGRES_URI ##example"postgres://user:pass@host/dbname"

GOFMT_FILES?=$$(find . -name '*.go' | grep -v vendor | grep -v yacc | grep -v .git)

.PHONY: all imports fmt test test-bridge-sdk clean-bridge-sdk run run-memory run-postgres run-valkey stop stop-memory stop-postgres stop-valkey clean clean-memory clean-postgres clean-valkey logs logs-memory logs-postgres logs-valkey status status-memory status-postgres status-valkey help

STORAGE ?= memory
DOCKER_COMPOSE_FILE = docker-compose.memory.yml

# Set compose file based on storage type
ifeq ($(STORAGE),memory)
    DOCKER_COMPOSE_FILE = docker-compose.memory.yml
else ifeq ($(STORAGE),postgres)
    DOCKER_COMPOSE_FILE = docker-compose.postgres.yml
else ifeq ($(STORAGE),valkey)
    DOCKER_COMPOSE_FILE = docker-compose.valkey.yml
endif

all: imports fmt test

build:
	go build -o bridge ./cmd/bridge

fmt:
	gofmt -w $(GOFMT_FILES)

fmtcheck:
	@sh -c "'$(CURDIR)/scripts/gofmtcheck.sh'"

lint:
	golangci-lint run --timeout=10m --color=always

test: 
	go test $$(go list ./... | grep -v /vendor/) -race -coverprofile cover.out

test-bridge-sdk:
	@./scripts/test-bridge-sdk.sh

clean-bridge-sdk:
	@echo "Cleaning up bridge-sdk directory..."
	@rm -rf bridge-sdk

run:
	@echo "Starting bridge environment with $(STORAGE) storage using $(DOCKER_COMPOSE_FILE)..."
	@if command -v docker-compose >/dev/null 2>&1; then \
		docker-compose -f $(DOCKER_COMPOSE_FILE) up --build -d; \
	elif command -v docker >/dev/null 2>&1; then \
		docker compose -f $(DOCKER_COMPOSE_FILE) up --build -d; \
	else \
		echo "Error: Docker is not installed or not in PATH"; \
		echo "Please install Docker Desktop from https://www.docker.com/products/docker-desktop"; \
		exit 1; \
	fi
	@echo "Environment started! Access the load bridge at http://localhost:8081"
	@echo "Use 'make logs' to view logs, 'make stop' to stop services"

run-memory:
	@$(MAKE) run STORAGE=memory

run-postgres:
	@$(MAKE) run STORAGE=postgres

run-valkey:
	@$(MAKE) run STORAGE=valkey

run-nginx:
	@$(MAKE) run STORAGE=postgres DOCKER_COMPOSE_FILE=docker-compose.nginx.yml

stop:
	@echo "Stopping bridge environment using $(DOCKER_COMPOSE_FILE)..."
	@if command -v docker-compose >/dev/null 2>&1; then \
		docker-compose -f $(DOCKER_COMPOSE_FILE) down; \
	elif command -v docker >/dev/null 2>&1; then \
		docker compose -f $(DOCKER_COMPOSE_FILE) down; \
	else \
		echo "Error: Docker is not installed or not in PATH"; \
		exit 1; \
	fi

stop-memory:
	@$(MAKE) stop STORAGE=memory

stop-postgres:
	@$(MAKE) stop STORAGE=postgres

stop-valkey:
	@$(MAKE) stop STORAGE=valkey

clean:
	@echo "Cleaning up bridge environment and volumes using $(DOCKER_COMPOSE_FILE)..."
	@if command -v docker-compose >/dev/null 2>&1; then \
		docker-compose -f $(DOCKER_COMPOSE_FILE) down -v --rmi local; \
	elif command -v docker >/dev/null 2>&1; then \
		docker compose -f $(DOCKER_COMPOSE_FILE) down -v --rmi local; \
	else \
		echo "Error: Docker is not installed or not in PATH"; \
		exit 1; \
	fi
	@if command -v docker >/dev/null 2>&1; then \
		docker system prune -f; \
	fi

clean-memory:
	@$(MAKE) clean STORAGE=memory

clean-postgres:
	@$(MAKE) clean STORAGE=postgres

clean-valkey:
	@$(MAKE) clean STORAGE=valkey

logs:
	@if command -v docker-compose >/dev/null 2>&1; then \
		docker-compose -f $(DOCKER_COMPOSE_FILE) logs -f; \
	elif command -v docker >/dev/null 2>&1; then \
		docker compose -f $(DOCKER_COMPOSE_FILE) logs -f; \
	else \
		echo "Error: Docker is not installed or not in PATH"; \
		exit 1; \
	fi

logs-memory:
	@$(MAKE) logs STORAGE=memory

logs-postgres:
	@$(MAKE) logs STORAGE=postgres

logs-valkey:
	@$(MAKE) logs STORAGE=valkey

status:
	@if command -v docker-compose >/dev/null 2>&1; then \
		docker-compose -f $(DOCKER_COMPOSE_FILE) ps; \
	elif command -v docker >/dev/null 2>&1; then \
		docker compose -f $(DOCKER_COMPOSE_FILE) ps; \
	else \
		echo "Error: Docker is not installed or not in PATH"; \
		exit 1; \
	fi

status-memory:
	@$(MAKE) status STORAGE=memory

status-postgres:
	@$(MAKE) status STORAGE=postgres

status-valkey:
	@$(MAKE) status STORAGE=valkey

help:
	@echo "Available storage backends:"
	@echo "  valkey   - Use Valkey (Redis-compatible) storage"
	@echo "  postgres - Use PostgreSQL storage"
	@echo "  memory   - Use in-memory storage (no persistence)"
	@echo ""
	@echo "Usage examples:"
	@echo "  make run-valkey           # Start with Valkey storage"
	@echo "  make run-postgres         # Start with PostgreSQL storage"
	@echo "  make run-memory           # Start with memory storage"
	@echo ""
	@echo "Other commands with storage-specific suffixes:"
	@echo "  make stop-valkey"
	@echo "  make logs-postgres"
	@echo "  make clean-memory"

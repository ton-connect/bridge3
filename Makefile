GOFMT_FILES?=$$(find . -name '*.go' | grep -v vendor | grep -v yacc | grep -v .git)

.PHONY: all imports fmt test test-bridge-sdk clean-bridge-sdk test-all integration-test run stop clean logs status

all: imports fmt test

build:
	go build -o callmebridge ./cmd/bridge

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

test-all: test test-bridge-sdk
	@echo "All tests completed successfully!"

integration-test:
	@./scripts/integration-test.sh

run:
	@echo "Starting bridge environment with nginx load balancer and 3 bridge instances..."
	@if command -v docker-compose >/dev/null 2>&1; then \
		docker-compose up --build -d; \
	elif command -v docker >/dev/null 2>&1; then \
		docker compose up --build -d; \
	else \
		echo "Error: Docker is not installed or not in PATH"; \
		echo "Please install Docker Desktop from https://www.docker.com/products/docker-desktop"; \
		exit 1; \
	fi
	@echo "Environment started! Access the load balancer at http://localhost:8080"
	@echo "Use 'make logs' to view logs, 'make stop' to stop services"

stop:
	@echo "Stopping bridge environment..."
	@if command -v docker-compose >/dev/null 2>&1; then \
		docker-compose down; \
	elif command -v docker >/dev/null 2>&1; then \
		docker compose down; \
	else \
		echo "Error: Docker is not installed or not in PATH"; \
		exit 1; \
	fi

clean:
	@echo "Cleaning up bridge environment and volumes..."
	@if command -v docker-compose >/dev/null 2>&1; then \
		docker-compose down -v --rmi local; \
	elif command -v docker >/dev/null 2>&1; then \
		docker compose down -v --rmi local; \
	else \
		echo "Error: Docker is not installed or not in PATH"; \
		exit 1; \
	fi
	@if command -v docker >/dev/null 2>&1; then \
		docker system prune -f; \
	fi

logs:
	@if command -v docker-compose >/dev/null 2>&1; then \
		docker-compose logs -f; \
	elif command -v docker >/dev/null 2>&1; then \
		docker compose logs -f; \
	else \
		echo "Error: Docker is not installed or not in PATH"; \
		exit 1; \
	fi

status:
	@if command -v docker-compose >/dev/null 2>&1; then \
		docker-compose ps; \
	elif command -v docker >/dev/null 2>&1; then \
		docker compose ps; \
	else \
		echo "Error: Docker is not installed or not in PATH"; \
		exit 1; \
	fi

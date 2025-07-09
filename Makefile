GOFMT_FILES?=$$(find . -name '*.go' | grep -v vendor | grep -v yacc | grep -v .git)

.PHONY: all imports fmt test test-bridge-sdk clean-bridge-sdk test-all integration-test

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


.PHONY: all imports fmt test

all: imports fmt test

build:
	go build -o callmebridge ./

fmt:
	gofmt -w $(GOFMT_FILES)

fmtcheck:
	@sh -c "'$(CURDIR)/scripts/gofmtcheck.sh'"

lint:
	golangci-lint run --timeout=10m --color=always

test: 
	go test $$(go list ./... | grep -v /vendor/) -race -coverprofile cover.out

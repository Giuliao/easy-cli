GO ?= go
BINARY ?= easy
MAIN_PACKAGE := ./cmd/easy

.PHONY: all build install test test-race vet fmt fmt-check check run clean help

all: check build

build:
	$(GO) build -o $(BINARY) $(MAIN_PACKAGE)

install:
	$(GO) install $(MAIN_PACKAGE)

test:
	GOMAXPROCS=2 $(GO) test -p 2 ./...

test-race:
	GOMAXPROCS=2 $(GO) test -race -p 2 ./...

vet:
	GOMAXPROCS=2 $(GO) vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.git/*')

fmt-check:
	@test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './.git/*'))" || { echo 'gofmt check failed; run make fmt'; exit 1; }

check: fmt-check vet test

run:
	$(GO) run $(MAIN_PACKAGE) $(ARGS)

clean:
	rm -f $(BINARY)
	rm -rf bin dist coverage.out coverage.html

help:
	@printf '%s\n' \
		'make build       Build the easy CLI' \
		'make install     Install the easy CLI with go install' \
		'make test        Run all tests' \
		'make test-race   Run tests with the race detector' \
		'make vet         Run go vet' \
		'make fmt         Format Go files' \
		'make check       Run formatting check, vet, and tests' \
		'make run ARGS=... Run the CLI' \
		'make clean       Remove local build artifacts'

BINARY := agentbox
PKG := ./cmd/agentbox
WEB := internal/gui/web
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: all build web-install web-build web-test test test-go vet race cover test-live clean

all: build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

web-install:
	npm --prefix $(WEB) ci

web-build: web-install
	npm --prefix $(WEB) run build

web-test: web-install
	npm --prefix $(WEB) run test

test: test-go web-test

test-go:
	go test ./...

vet:
	go vet ./...

race:
	go test -race ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

test-live:
	AGENTBOX_LIVE=1 go test -tags live -count=1 -v -run '^TestLive' ./it

clean:
	rm -f $(BINARY) coverage.out $(WEB)/tsconfig.tsbuildinfo

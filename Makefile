BINARY := agentbox
PKG := ./cmd/agentbox
WEB := internal/gui/web
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: all build web-install web-build web-test test test-go smoke-cli vet race cover test-live clean

all: build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

web-install:
	@internal/gui/web/scripts/npm-ci.sh $(WEB)

web-build: web-install
	npm --prefix $(WEB) run build

web-test: web-install
	npm --prefix $(WEB) run test

test: test-go web-test smoke-cli

test-go:
	go test ./...

smoke-cli:
	@set -eu; \
	assert_line() { \
		printf '%s\n' "$$1" | grep -Fx -- "$$2"; \
	}; \
	smoke_home="$(CURDIR)/.agentbox-smoke"; \
	trap 'rm -rf "$$smoke_home"' EXIT HUP INT TERM; \
	rm -rf "$$smoke_home"; \
	mkdir -p "$$smoke_home"; \
	export AGENTBOX_HOME="$$smoke_home"; \
	export PATH="$(CURDIR)/cmd/agentbox/testdata/fake-openshell:$$PATH"; \
	default_text=$$(go run $(PKG) provider list); \
	assert_line "$$default_text" 'openai               openai'; \
	assert_line "$$default_text" 'anthropic            anthropic'; \
	team_text=$$(go run $(PKG) provider list --gateway team-gateway); \
	assert_line "$$team_text" 'team-openai          openai-compatible'; \
	assert_line "$$team_text" 'team-anthropic       anthropic-compatible'; \
	default_json=$$(go run $(PKG) provider list --json); \
	assert_line "$$default_json" '{"providers":[{"name":"openai","type":"openai"},{"name":"anthropic","type":"anthropic"}]}'; \
	team_json=$$(go run $(PKG) provider list --json --gateway team-gateway); \
	assert_line "$$team_json" '{"providers":[{"name":"team-openai","type":"openai-compatible"},{"name":"team-anthropic","type":"anthropic-compatible"}]}'

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

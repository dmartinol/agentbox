BINARY := agentbox
PKG := ./cmd/agentbox

.PHONY: all build test vet race cover clean

all: build

build:
	go build -o $(BINARY) $(PKG)

test:
	go test ./...

vet:
	go vet ./...

race:
	go test -race ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

clean:
	rm -f $(BINARY) coverage.out

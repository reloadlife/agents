MODULE   := github.com/reloadlife/agents
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE     ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
PREFIX   ?= $(HOME)/.local

.PHONY: all build install test fmt vet tidy clean run-daemon smoke release-check

all: build

build:
	mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/agentsd ./cmd/agentsd
	go build -ldflags "$(LDFLAGS)" -o bin/agentsctl ./cmd/agentsctl

install: build
	mkdir -p "$(PREFIX)/bin"
	install -m 755 bin/agentsd bin/agentsctl "$(PREFIX)/bin/"
	@echo "installed to $(PREFIX)/bin"

test:
	go test ./...

# Requires tmux; used by CI
test-integration:
	go test -tags=integration -count=1 -timeout 2m ./internal/session/

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.git/*')

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -rf bin dist .jobs

# Local smoke (requires tmux for session tests)
run-daemon: build
	mkdir -p .jobs
	AGENTSD_TOKEN=$${AGENTSD_TOKEN:-dev-token} \
	  ./bin/agentsd serve --config config.example.toml

smoke: build
	@AGENTSD_TOKEN=dev-token ./bin/agentsd serve --config config.example.toml & echo $$! > /tmp/agentsd-smoke.pid; \
	sleep 0.5; \
	./bin/agentsctl --url http://127.0.0.1:8787 --token dev-token status >/dev/null && \
	./bin/agentsctl --url http://127.0.0.1:8787 --token dev-token agents >/dev/null && \
	echo "smoke ok"; \
	kill $$(cat /tmp/agentsd-smoke.pid) 2>/dev/null || true

release-check: vet test build
	@echo "version=$(VERSION) ok"

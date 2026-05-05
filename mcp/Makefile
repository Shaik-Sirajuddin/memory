ADDR ?= :8100
INTERVAL ?= 1m
STDIO_INTERVAL ?= 10s
PID_FILE ?= .mcp.pid
LOG_FILE ?= .mcp.log
GOCACHE ?= /tmp/go-build-mcp
BIN ?= .bin/mcp

.PHONY: build run run-stdio start stop restart status test

build:
	GOCACHE=$(GOCACHE) go build -o $(BIN) ./cmd/mcp

run: build
	$(BIN) -addr $(ADDR) -interval $(INTERVAL)

run-stdio:
	GOCACHE=$(GOCACHE) go run ./cmd/mcp -transport stdio -interval $(STDIO_INTERVAL)

start: build
	@if [ -f "$(PID_FILE)" ] && kill -0 "$$(cat $(PID_FILE))" 2>/dev/null; then \
		echo "mcp already running: $$(cat $(PID_FILE))"; \
	else \
		nohup $(BIN) -addr $(ADDR) -interval $(INTERVAL) >$(LOG_FILE) 2>&1 & echo $$! > $(PID_FILE); \
		echo "mcp started: $$(cat $(PID_FILE))"; \
	fi

stop:
	@if [ -f "$(PID_FILE)" ] && kill -0 "$$(cat $(PID_FILE))" 2>/dev/null; then \
		kill "$$(cat $(PID_FILE))"; \
		rm -f $(PID_FILE); \
		echo "mcp stopped"; \
	else \
		rm -f $(PID_FILE); \
		echo "mcp not running"; \
	fi

restart: stop start

status:
	@if [ -f "$(PID_FILE)" ] && kill -0 "$$(cat $(PID_FILE))" 2>/dev/null; then \
		echo "mcp running: $$(cat $(PID_FILE))"; \
	else \
		echo "mcp not running"; \
	fi

test:
	GOCACHE=$(GOCACHE) go test -tags unit ./...

# =========================
# Targets
# =========================

.PHONY: build-ptydaemon
build-ptydaemon:
	cd ../svc/ptydaemon && go build -o omni-server ./cmd/

.PHONY: build
build: build-ptydaemon

.PHONY: install
install:
	go install ./cli/cmd/omni/...
	cd ../svc/ptydaemon && go build -o $(shell go env GOPATH)/bin/omni-server ./cmd/

.PHONY: test-unit
test-unit:
	go test -tags=unit ./...

.PHONY: test-integration
test-integration:
	go test -tags=integration ./...

.PHONY: test-codex-integration
test-codex-integration:
	go test -tags=integration ./connector/codeagent/codex/...

.PHONY: generate-schema
generate-schema:
	$(MAKE) -C connector/codeagent/claude gen-structs-from-schema
	$(MAKE) -C connector/codeagent/codex gen-structs-from-schema
	$(MAKE) -C connector/codeagent/gemini gen-structs-from-schema

.PHONY: generate-schema-from-structs
generate-schema-from-structs:
	$(MAKE) -C connector/codeagent/claude gen-schema-from-structs
	$(MAKE) -C connector/codeagent/gemini gen-schema-from-structs
# Clean generated file
.PHONY: clean
clean:
	rm -f $(OUTPUT_FILE)
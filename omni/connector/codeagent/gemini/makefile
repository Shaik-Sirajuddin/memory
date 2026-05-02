# =========================
# Config
# =========================
SCHEMA_URL := https://raw.githubusercontent.com/google-gemini/gemini-cli/main/schemas/settings.schema.json
OUTPUT_FILE := ./settings.gen.go
PACKAGE_PATH := github.com/Shaik-Sirajuddin/memory/connector/codeagent/gemini
SCHEMA_FILE := ./cli/settings_schema.json

GO_JSONSCHEMA := go-jsonschema

# =========================
# Targets
# =========================

.PHONY: gen-structs-from-schema
gen-structs-from-schema:
	$(GO_JSONSCHEMA) \
		--schema-package=$(SCHEMA_URL)=$(PACKAGE_PATH) \
		--schema-output=$(SCHEMA_URL)=$(OUTPUT_FILE) \
		$(SCHEMA_FILE)

.PHONY: gen-schema-from-structs
gen-schema-from-structs:
	go run ./gen-schema > $(SCHEMA_FILE)

# Clean generated file
.PHONY: clean
clean:
	rm -f $(OUTPUT_FILE)
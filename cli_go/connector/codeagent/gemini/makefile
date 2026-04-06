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

.PHONY: generate-schema
gen-structs-from-schema:
	$(GO_JSONSCHEMA) \
		--schema-package=$(SCHEMA_URL)=$(PACKAGE_PATH) \
		--schema-output=$(SCHEMA_URL)=$(OUTPUT_FILE) \
		$(SCHEMA_FILE)
# Clean generated file
.PHONY: clean
clean:
	rm -f $(OUTPUT_FILE)
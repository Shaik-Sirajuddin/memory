# =========================
# Config
# =========================
OUTPUT_FILE := ./settings.gen.go
PACKAGE_NAME := codex
SCHEMA_FILE := ./spec/settings_schema.json

GO_JSONSCHEMA := go-jsonschema

# =========================
# Targets
# =========================

.PHONY: gen-structs-from-schema
gen-structs-from-schema:
	$(GO_JSONSCHEMA) \
		-p $(PACKAGE_NAME) \
		-o $(OUTPUT_FILE) \
		$(SCHEMA_FILE)
# Clean generated file
.PHONY: clean
clean:
	rm -f $(OUTPUT_FILE)
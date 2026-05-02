# =========================
# Targets
# =========================

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
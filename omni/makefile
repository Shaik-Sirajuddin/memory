# =========================
# Targets
# =========================

.PHONY: generate-schema
generate-schema:
	$(MAKE) -C connector/codeagent/claude gen-structs-from-schema
	$(MAKE) -C connector/codeagent/codex gen-structs-from-schema
	$(MAKE) -C connector/codeagent/gemini gen-structs-from-schema
# Clean generated file
.PHONY: clean
clean:
	rm -f $(OUTPUT_FILE)
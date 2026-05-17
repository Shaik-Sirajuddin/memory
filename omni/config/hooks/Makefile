SCHEMA_FILE := ./spec/hooks_schema.json

.PHONY: export-hooks-schema
export-hooks-schema:
	mkdir -p spec
	go run ./gen-schema > $(SCHEMA_FILE)

.PHONY: clean
clean:
	rm -f $(SCHEMA_FILE)

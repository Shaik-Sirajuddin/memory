SCHEMA_FILE := ./spec/config_schema.json

.PHONY: export-config-schema
export-config-schema:
	mkdir -p spec
	go run ./gen-schema > $(SCHEMA_FILE)
	$(MAKE) -C hooks export-hooks-schema

.PHONY: clean
clean:
	rm -f $(SCHEMA_FILE)

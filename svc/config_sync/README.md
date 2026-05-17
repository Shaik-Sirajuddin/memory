# Config Sync Service

Draft implementation.

This package contains the initial `config_sync` service for synchronizing omni hook configuration into active agent hook transformers. It is currently scoped to the service and registry implementation only; runtime wiring through operator, CLI, server startup, and the separate hook operator service is still pending.

Source config is read through `config.OmniConfigResolver`, which defaults to the XDG user config path for `omni/config.json`.

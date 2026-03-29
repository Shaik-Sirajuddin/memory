# Architecture Overview

The service is split into a few small layers so the auth logic can stay independent from HTTP and storage details.

## Layers

- `cmd/authsvc`
  - process entrypoint
  - lifecycle handling

- `internal/app`
  - wires config, auth, keys, cache, repo, and HTTP layers together

- `internal/httpapi`
  - exposes routes
  - applies session/JWT/RBAC middleware
  - performs proxy forwarding

- `internal/auth`
  - account signup/login
  - session issuance
  - JWT signing and verification

- `internal/keys`
  - service-account lifecycle
  - signing-token flow
  - JWT issuance for service accounts

- `internal/repo`
  - persistence boundary
  - in-memory implementation today

- `internal/cache`
  - cache and rate-limit boundary
  - in-memory implementation today

## Data Flow

```text
Client -> HTTP API -> auth/keys services -> repo/cache -> response
Client -> HTTP API -> JWT/RBAC middleware -> reverse proxy -> upstream
```

## Current Tradeoff

The repository currently runs with in-memory backing implementations so it can build and test without external services. The interfaces are already separated so PostgreSQL and Redis adapters can be added later without changing the HTTP surface.


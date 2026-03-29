# Networking

This document describes the request flow and network behavior of the service.

## Listening Port

- Default port: `4000`

## External Surfaces

- `GET /health`
- `POST /auth/signup`
- `POST /auth/login`
- `GET /auth/oauth/{provider}/callback`
- `POST /keys/service-account/create`
- `PUT /keys/service-account/update`
- `DELETE /keys/service-account/delete`
- `POST /keys/initiate-auth`
- `POST /keys/authenticate`
- `GET /store/get`

## Proxy Behavior

The `/store/*` routes are forwarded to the configured upstream base URL.

Forwarding rules:
- preserve the request body stream
- keep normal client headers unless they are internal-only
- strip internal headers such as `X-Internal-Auth` and `X-Internal-Token`
- forward only after JWT and RBAC checks pass

## Authorization Headers

- `Authorization: Bearer <session_token>` for `/keys/*`
- `Authorization: Bearer <jwt>` for `/store/*`

## RBAC Decision

The proxy authorizes a request when:
- the JWT is valid
- the token state exists in cache
- the method maps to the required scope
- the request path matches a configured scope glob

## Suggested Deployment Notes

- Put the service behind a reverse proxy or ingress controller.
- Terminate TLS before the service or in the service gateway.
- Route upstream service traffic only through the authenticated proxy path if the service is the policy enforcement point.


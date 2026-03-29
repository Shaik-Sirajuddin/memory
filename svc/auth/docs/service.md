# Service Usage Guide

This guide shows how to use the auth service from another party or client.

## Base URLs

- Local default: `http://localhost:4000`
- Proxy target: configured by `UPSTREAM_BASE_URL`

## Authentication Modes

The service uses two token types:

1. Session token
- Returned by `/auth/signup`, `/auth/login`, and OAuth callback flows
- Sent to `/keys/*` endpoints as `Authorization: Bearer <session_token>`

2. JWT access token
- Returned by `/keys/authenticate`
- Sent to `/store/*` endpoints as `Authorization: Bearer <jwt>`

## Environment Variables

| Variable | Purpose |
|---|---|
| `PORT` | HTTP listen port |
| `DATABASE_URL` | Database connection string, if an external store is used |
| `REDIS_URL` | Redis connection string, if an external cache is used |
| `JWT_SECRET` | Signing secret for JWT issuance |
| `JWT_TTL_SECONDS` | JWT lifetime |
| `SIGNING_TOKEN_TTL_SECONDS` | Signing-token lifetime |
| `SESSION_TTL_SECONDS` | Session lifetime |
| `GOOGLE_CLIENT_ID` | Google OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | Google OAuth client secret |
| `GOOGLE_CALLBACK_URL` | Google callback URL |
| `GITHUB_CLIENT_ID` | GitHub OAuth client ID |
| `GITHUB_CLIENT_SECRET` | GitHub OAuth client secret |
| `GITHUB_CALLBACK_URL` | GitHub callback URL |
| `ORG_DOMAIN_WHITELIST` | Comma-separated domains treated as team accounts |
| `UPSTREAM_BASE_URL` | Upstream service for `/store` forwarding |

## Auth Endpoints

### `POST /auth/signup`

Create an account with email/password or OAuth onboarding.

Example request:

```json
{
  "email": "ada@acme.com",
  "password": "password123"
}
```

OAuth-style request:

```json
{
  "provider": "google",
  "token": "mock:ada@acme.com:google-user-123"
}
```

Response:

```json
{
  "uid": "uuid",
  "email": "ada@acme.com",
  "account_type": "team",
  "organization": "acme.com",
  "session_token": "sess_..."
}
```

### `POST /auth/login`

Use the same shape as signup for either password or provider-based login.

### `GET /auth/oauth/{provider}/callback`

Supported providers:
- `google`
- `github`

Query inputs:
- `code`
- `token`
- `email`
- `state`

The current implementation resolves provider identity from the query parameters so the flow is easy to exercise locally.

## Keys Endpoints

All `/keys/*` routes require a session token in the `Authorization` header.

### `POST /keys/service-account/create`

Create a service account and attach scopes.

Example:

```json
{
  "name": "CI runner",
  "secret_key": "my-secret",
  "expiry_date": "2026-12-31",
  "scopes": [
    {
      "root": "team",
      "path": "/store/*",
      "scope": "read"
    }
  ]
}
```

### `PUT /keys/service-account/update`

Replace the service account metadata and scope list in full.

### `DELETE /keys/service-account/delete?uid=<service-account-uid>`

Deletes a service account by UID.

### `POST /keys/initiate-auth`

Start the two-step signing flow.

Request:

```json
{
  "service_account_token": "sa_..."
}
```

Response:

```json
{
  "token": "st_..."
}
```

### `POST /keys/authenticate`

Finish the two-step signing flow and issue a short-lived JWT.

Request:

```json
{
  "token": "st_...",
  "signedPayload": "<hmac-hex>"
}
```

Response:

```json
{
  "jwt": "<short-lived-jwt>"
}
```

## Store Proxy

All `/store/*` routes require a JWT bearer token.

### `GET /store/get`

Example:

```bash
curl -H "Authorization: Bearer $JWT" http://localhost:4000/store/get
```

The proxy:
- validates the JWT signature
- checks the token state in cache
- verifies the request path against the service-account scopes
- forwards the request to `UPSTREAM_BASE_URL`
- strips internal headers before forwarding

## Common Flow

1. Sign up or log in.
2. Create a service account.
3. Call `POST /keys/initiate-auth`.
4. Sign the returned token.
5. Call `POST /keys/authenticate` to get a JWT.
6. Call `/store/*` with the JWT.


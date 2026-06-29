# orderflow

A small but production-shaped **order management REST API** written in Go.

It is intentionally scoped like a take-home/backend assignment: users authenticate
with JWTs, browse a product catalog, and place orders whose stock is reserved
**transactionally and race-safely**. The goal is to show how a real service is
structured — clean layering, dependency inversion, meaningful tests, migrations,
containerization, and CI — rather than to be feature-complete.

> Built with the Go **standard library** for routing (`net/http`, Go 1.22 mux) and
> structured logging (`log/slog`), plus two well-known libraries: `lib/pq` for
> Postgres and `golang-jwt` for tokens.

---

## Features

- **Auth** — register / login, password hashing, JWT (HS256) with role claims.
- **Catalog** — list/read products (public); create/update/delete (admin only).
- **Orders** — place an order across multiple products; per-user listing; admin
  status transitions enforced by a small state machine.
- **Race-safe stock** — concurrent orders for the last unit can never oversell.
- **Validation** — structured `422` responses listing the offending fields.
- **Operability** — health endpoint, request logging, graceful shutdown, tuned
  HTTP timeouts and DB pool.
- **Tested** — unit tests (no infra needed) plus DB integration tests behind a
  build tag, both run in CI.

## Tech stack

| Concern        | Choice                                            |
| -------------- | ------------------------------------------------- |
| Language       | Go 1.22                                           |
| HTTP routing   | stdlib `net/http` (1.22 method + wildcard mux)    |
| Database       | PostgreSQL via `database/sql` + `lib/pq`          |
| Auth           | `golang-jwt/jwt/v5` (HS256), PBKDF2 password hash |
| Logging        | stdlib `log/slog` (JSON in prod, text in dev)     |
| Migrations     | embedded SQL, applied on startup                  |
| Container      | multi-stage build → distroless, non-root          |
| CI             | GitHub Actions (unit + integration with Postgres) |

---

## Architecture

The code is organized in layers with a strict **inward** dependency direction.
Each layer depends only on the abstractions of the layer beneath it, and those
abstractions (interfaces) are declared by the **consumer**, not the implementer.

```
                 HTTP request
                      │
              ┌───────▼────────┐
              │    httpapi     │  routing, JWT middleware, JSON (de)coding,
              │   (transport)  │  domain-error → HTTP-status mapping
              └───────┬────────┘
                      │  depends on service interfaces
              ┌───────▼────────┐
              │    service     │  business rules, validation, authorization,
              │  (use cases)   │  price snapshot + total calculation
              └───────┬────────┘
                      │  depends on repository interfaces
              ┌───────▼────────┐
              │   repository   │  Postgres queries, transactions,
              │ (persistence)  │  conditional stock reservation
              └───────┬────────┘
                      │
                 ┌────▼────┐
                 │ Postgres│
                 └─────────┘

   domain  ─ shared entities + sentinel errors; no dependencies
   auth    ─ password hashing (PBKDF2) and JWT issue/verify
```

Why this matters: because `service` defines the repository interfaces it needs,
the business logic is unit-tested against in-memory fakes with no database, and
Postgres is an implementation detail that could be swapped without touching the
rules. The same applies one layer up: handlers are tested against fake services.

### Project structure

```
orderflow/
├── cmd/api/                 # main(): config, wiring, server lifecycle
├── internal/
│   ├── config/              # env-based configuration + validation
│   ├── domain/              # entities, the order state machine, sentinel errors
│   ├── auth/                # PBKDF2 password hashing, JWT manager
│   ├── database/            # connection (with retry) + migration runner
│   ├── repository/          # Postgres implementations (+ integration tests)
│   ├── service/             # business logic (+ unit tests with fakes)
│   └── httpapi/             # router, middleware, handlers (+ httptest tests)
├── migrations/              # embedded .sql migrations
├── api/openapi.yaml         # OpenAPI 3 spec
├── .github/workflows/ci.yml # build, vet, unit + integration tests
├── Dockerfile
├── docker-compose.yml
└── Makefile
```

---

## Getting started

> **Note:** the module path is `github.com/yourusername/orderflow`. Replace
> `yourusername` (in `go.mod` and the imports) with your GitHub handle before
> pushing — a quick find-and-replace.

### Run the whole stack with Docker

```bash
docker compose up --build
```

This starts Postgres and the API, waits for the database to be healthy, applies
migrations automatically, and serves on **http://localhost:8080**.

### Run locally against your own Postgres

```bash
cp .env.example .env          # then export the vars, or use a tool like direnv
export $(grep -v '^#' .env | xargs)
make run
```

The server applies migrations on startup, so there is no separate migrate step.

---

## Configuration

All configuration comes from the environment (12-factor).

| Variable       | Required | Default       | Description                          |
| -------------- | -------- | ------------- | ------------------------------------ |
| `DATABASE_URL` | yes      | —             | Postgres DSN                         |
| `JWT_SECRET`   | yes      | —             | HMAC signing key (min 16 chars)      |
| `PORT`         | no       | `8080`        | HTTP listen port                     |
| `ENV`          | no       | `development` | `production` switches to JSON logs   |
| `JWT_TTL`      | no       | `24h`         | Token lifetime (Go duration)         |

---

## API reference

Full schema in [`api/openapi.yaml`](api/openapi.yaml).

| Method   | Path                   | Auth   | Description                  |
| -------- | ---------------------- | ------ | --------------------------- |
| `GET`    | `/healthz`             | —      | Liveness check              |
| `POST`   | `/auth/register`       | —      | Create an account           |
| `POST`   | `/auth/login`          | —      | Get a JWT                   |
| `GET`    | `/products`            | —      | List products               |
| `GET`    | `/products/{id}`       | —      | Get a product               |
| `POST`   | `/products`            | admin  | Create a product            |
| `PUT`    | `/products/{id}`       | admin  | Replace a product           |
| `DELETE` | `/products/{id}`       | admin  | Delete a product            |
| `POST`   | `/orders`              | user   | Place an order              |
| `GET`    | `/orders`              | user   | List your orders            |
| `GET`    | `/orders/{id}`         | user   | Get an order (owner/admin)  |
| `PATCH`  | `/orders/{id}/status`  | admin  | Change order status         |

### Example flow

```bash
# 1. Register and capture the token
curl -s localhost:8080/auth/register \
  -d '{"email":"alice@example.com","password":"supersecret"}'

TOKEN=$(curl -s localhost:8080/auth/login \
  -d '{"email":"alice@example.com","password":"supersecret"}' \
  | sed -E 's/.*"token":"([^"]+)".*/\1/')

# 2. Place an order (assuming product 1 exists with stock)
curl -s localhost:8080/orders \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"items":[{"product_id":1,"quantity":2}]}'
```

Money is represented everywhere as integer **cents** (`price_cents`,
`unit_price_cents`, `total_cents`) to avoid floating-point rounding.

---

## Testing

```bash
make test-race          # unit tests with the race detector (no DB required)
make test-integration   # repository tests against a real Postgres
```

- **Unit tests** cover password hashing (against a known PBKDF2 vector), JWT
  edge cases (expiry, tampering, `alg=none`), all service business rules, and
  the HTTP layer (routing, auth/admin middleware, status mapping) — all with
  fakes, so they run anywhere.
- **Integration tests** (`//go:build integration`) exercise the real SQL,
  including a concurrency test that fires 20 simultaneous orders at a
  single-unit product and asserts **exactly one** succeeds.

---

## Design decisions

**Layering with consumer-defined interfaces.** Interfaces live with the code
that *uses* them (`service` declares its repositories; `httpapi` declares its
services). This is what keeps each layer testable in isolation and the
dependencies pointing inward.

**Transactional, race-safe stock reservation.** Placing an order reserves stock
with a single conditional statement per line:

```sql
UPDATE products SET stock = stock - $qty WHERE id = $id AND stock >= $qty
```

Because the check and the decrement are one atomic operation, two concurrent
orders for the last unit cannot both succeed — the loser sees zero rows affected
and the whole transaction rolls back with `insufficient stock`. The service
layer also does an early, friendly stock check, but it is explicitly *not* the
guard against overselling; the database is the source of truth. This is the
project's most important correctness property and is verified by an integration
test.

**Price snapshots.** Each order line stores the product price at purchase time
(`unit_price_cents`), so later catalog price changes never rewrite history.

**Password hashing with PBKDF2-HMAC-SHA256.** Hashing uses PBKDF2 (RFC 8018)
over the standard library's HMAC, with an OWASP-recommended iteration count, a
random per-password salt, a self-describing encoded format (so the cost can be
raised later without invalidating old hashes), and constant-time comparison.
PBKDF2 is **FIPS-140 approved**, which is the main reason it is chosen here over
bcrypt; argon2id is an equally sound choice where FIPS is not a constraint. The
implementation is checked against an independently generated test vector.

**JWT with a pinned algorithm.** Tokens are HS256 and the verifier *only* accepts
HS256 — it never trusts the algorithm advertised in the token header — which
closes the classic `alg=none` / algorithm-confusion attacks. Expiry and issuer
are validated too.

**Errors as a transport-agnostic vocabulary.** The service and repository layers
speak in domain sentinel errors (`ErrNotFound`, `ErrConflict`,
`ErrInsufficientStock`, …) and structured `FieldErrors`. A single mapping in the
HTTP layer turns those into status codes, so business code carries no knowledge
of HTTP.

**No user enumeration.** Login returns the same error for an unknown email and a
wrong password; reading someone else's order returns `404`, not `403`.

## Possible extensions

A few things deliberately left out, to keep scope tight:

- Restock on order cancellation (a transactional reverse of the reservation).
- Refresh tokens / token revocation.
- Cursor-based pagination and richer product filtering.
- Idempotency keys on order creation.

## License

MIT — see `LICENSE` (add one when you publish).

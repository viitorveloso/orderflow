# --- build stage ------------------------------------------------------------
FROM golang:1.22-alpine AS builder

WORKDIR /src

# Download dependencies first so this layer is cached unless go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

# Build a static binary (CGO disabled; lib/pq is pure Go).
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/api ./cmd/api

# --- runtime stage ----------------------------------------------------------
# distroless/static has no shell or package manager, shrinking the attack
# surface, and runs as a non-root user by default.
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=builder /out/api /app/api

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/api"]

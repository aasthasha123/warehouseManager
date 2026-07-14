# ---- build stage ----
FROM golang:1.22-alpine AS build
WORKDIR /src

# Cache module downloads separately from source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO_ENABLED=0: lib/pq is pure Go, so this produces a fully static
# binary with no libc dependency — it can run on the minimal image below.
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/coldstorage-server ./cmd/server

# ---- run stage ----
FROM alpine:3.20
# ca-certificates: needed to verify TLS certs if you ever move from
# sslmode=require to sslmode=verify-full against your Postgres host.
RUN apk add --no-cache ca-certificates && \
    adduser -D -H -u 10001 appuser
USER appuser

COPY --from=build /out/coldstorage-server /coldstorage-server

EXPOSE 8080
ENTRYPOINT ["/coldstorage-server"]

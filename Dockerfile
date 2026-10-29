# One Dockerfile for all three binaries; pick which one with --build-arg CMD=...
# (server | mock-sales | mock-service). A separate "migrate" target runs goose.

# ---- build ----
FROM golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG CMD=server
RUN CGO_ENABLED=0 go build -o /out/app ./cmd/${CMD}

# ---- migrate: one-shot container that applies migrations, then exits ----
FROM golang:1.26-alpine AS goose
RUN CGO_ENABLED=0 go install github.com/pressly/goose/v3/cmd/goose@v3.28.0

FROM alpine:3.22 AS migrate
COPY --from=goose /go/bin/goose /usr/local/bin/goose
COPY migration /migration
ENTRYPOINT ["sh", "-c", "goose -dir /migration postgres \"$DATABASE_URL\" up"]

# ---- runtime (default target, keep it last) ----
FROM alpine:3.22
# -D: no password, -H: no home directory
RUN adduser -D -H app
COPY --from=build /out/app /app
USER app
ENTRYPOINT ["/app"]

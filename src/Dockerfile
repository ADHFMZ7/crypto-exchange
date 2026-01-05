# ---------- build stage ----------
FROM golang:1.24-alpine AS build

WORKDIR /app

# Install CA certs for HTTPS (pgx, TLS, etc.)
RUN apk add --no-cache ca-certificates

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o app ./cmd/main.go


# ---------- runtime stage ----------
FROM gcr.io/distroless/base-debian12

WORKDIR /app

# Copy binary only
COPY --from=build /app/app /app/app

# If your app reads config files at runtime, uncomment:
# COPY --from=build /app/config /app/config

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/app/app"]

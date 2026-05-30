FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o gateway ./cmd/gateway

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/gateway .
COPY --from=builder /app/internal/config/config.yaml ./internal/config/config.yaml
COPY --from=builder /app/migrations ./migrations
EXPOSE 8080 50051
CMD ["./gateway"]

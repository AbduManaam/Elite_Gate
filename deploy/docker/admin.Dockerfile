
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o admin ./cmd/admin

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/admin .
COPY --from=builder /app/internal/config/config.yaml ./internal/config/config.yaml
COPY --from=builder /app/migrations ./migrations
EXPOSE 9090
CMD ["./admin"]


#multi-stage Docker build.
#The first stage, builder, is only for compiling the Go app.
#The second stage is the final runtime image, which stays small and clean.
#So there are two setups because one is for building the server, and the other is for running it. This makes the image lighter, faster to ship, and more secure.

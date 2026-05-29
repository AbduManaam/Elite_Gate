ARG SAMPLE_CMD=./cmd/samples/http-user

FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG SAMPLE_CMD
RUN CGO_ENABLED=0 GOOS=linux go build -o sample $SAMPLE_CMD

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/sample .
EXPOSE 8000
CMD ["./sample"]

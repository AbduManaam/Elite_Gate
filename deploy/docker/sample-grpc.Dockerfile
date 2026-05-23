FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /sample ./cmd/samples/grpc-hello

FROM alpine:3.20
COPY --from=builder /sample /sample
EXPOSE 50052
CMD ["/sample"]
FROM golang:1.25-alpine AS builder
WORKDIR /app
ARG SAMPLE_CMD
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /sample ${SAMPLE_CMD}

FROM alpine:3.20
COPY --from=builder /sample /sample
CMD ["/sample"]
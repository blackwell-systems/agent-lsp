# Multi-stage Dockerfile fixture for LSP testing
# Stage 1: Builder
FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o myapp main.go

# Stage 2: Runtime
FROM alpine:latest AS runtime

WORKDIR /root/

COPY --from=builder /app/myapp .

CMD ["./myapp"]

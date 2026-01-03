FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o feed-service ./cmd/main.go

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/feed-service .
COPY migrations ./migrations
EXPOSE 8084
CMD ["./feed-service"]

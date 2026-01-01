# Build Stage
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

# Run Stage
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/server .
# Copy migrations if needed (assuming using go-migrate or similar)
# COPY --from=builder /app/migrations ./migrations 
EXPOSE 8080
CMD ["./server"]

# Build Stage
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./api/cmd

# Run Stage
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/server .
# Email service typically consumes RMQ, no public port needed, but we expose 8080 for healthz
EXPOSE 8080 
CMD ["./server"]

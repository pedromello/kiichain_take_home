# Stage 1: Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

WORKDIR /app

# Copy go.mod and go.sum to cache dependency downloads
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the Go application statically with symbols stripped to optimize size
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o server ./cmd/server/main.go

# Stage 2: Final runtime stage
FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy only the compiled binary from the build stage
COPY --from=builder /app/server .

# Expose default application port
EXPOSE 8080

# Run the binary
ENTRYPOINT ["./server"]

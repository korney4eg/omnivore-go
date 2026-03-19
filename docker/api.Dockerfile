# Build stage
FROM golang:alpine AS builder

# Set Go toolchain to auto to allow newer versions
ENV GOTOOLCHAIN=auto

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git make

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the API
RUN go build -o bin/omnivore-api .

# Runtime stage
FROM alpine:latest

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Copy binary from builder
COPY --from=builder /app/bin/omnivore-api /app/omnivore-api

# Expose port
EXPOSE 8080

# Run the API
CMD ["/app/omnivore-api", "server", "api"]

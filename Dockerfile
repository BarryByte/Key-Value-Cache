# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy Go module files first to cache dependencies
COPY go.mod ./
RUN go mod download

# Copy the rest of the source code
COPY main.go ./

# Build the Go application
RUN go build -o kvcache main.go

# Runtime stage
FROM alpine:latest

WORKDIR /app

# Copy the compiled binary from the builder stage
COPY --from=builder /app/kvcache .

# Expose the application port
EXPOSE 7171

# Command to run the application
CMD ["./kvcache"]

# Build Stage
FROM golang:latest AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod tidy

COPY . .

# Build a statically linked binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ./bin/pushpop ./cmd/main.go

# Minimal Runtime Stage
FROM gcr.io/distroless/static-debian12

WORKDIR /

# Copy the statically built binary
COPY --from=builder /app/bin/pushpop ./pushpop

# Ensure the binary has executable permissions
RUN chmod +x ./pushpop

# Default command
CMD ["./pushpop"]

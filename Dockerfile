# Build Stage
FROM golang:latest AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod tidy

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o ./bin/pushpop ./cmd/main.go

# Minimal Runtime Stage
FROM gcr.io/distroless/static

WORKDIR /

# Copy the statically built binary
COPY --from=builder /app/bin/pushpop ./pushpop

EXPOSE 8945
ENTRYPOINT ["/pushpop"]

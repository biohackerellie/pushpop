FROM golang:1.23.4-apline AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod tidy

COPY . .

RUN go build -o ./bin/pushpop ./cmd/main.go

FROM alpine:latest

COPY --from=builder /app/bin/pushpop .

CMD ["./pushpop"]

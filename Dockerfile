FROM golang:latest AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod tidy

COPY . .

RUN go build -o ./bin/pushpop ./cmd/main.go

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app/bin/pushpop /

CMD ["/pushpop"]

build-go:
	CGO_ENABLED=1 go build -o ./bin/pushpop ./cmd/main.go

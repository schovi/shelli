.PHONY: build run lint test test-cover install clean security

build:
	go build -o shelli .

run:
	go run .

lint:
	golangci-lint run

test:
	go test -v -race ./...

test-cover:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

install:
	go install .

clean:
	rm -f shelli coverage.out coverage.html

security:
	gosec -exclude=G104,G204,G301,G302,G304,G306 ./...
	govulncheck ./...

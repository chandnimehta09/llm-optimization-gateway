.PHONY: build run test docker-build docker-up

build:
	go build -o bin/gateway ./cmd/gateway

run:
	go run ./cmd/gateway

test:
	go test ./...

docker-build:
	docker build -t llm-optimization-gateway .

docker-up:
	docker-compose up --build

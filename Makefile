.PHONY: build run test migrate-build migrate-up migrate-down docker-test docker-build docker-up docker-down clean

build:
	@mkdir -p bin
	@go build -buildvcs=false -o bin/api ./cmd/api

run: build
	@./bin/api

test:
	@go test ./...

migrate-build:
	@mkdir -p bin
	@go build -buildvcs=false -o bin/migrate ./cmd/migrate

migrate-up: migrate-build
	@./bin/migrate up

migrate-down: migrate-build
	@./bin/migrate down

docker-test:
	@docker build --target test -t social-studio-api:test .

docker-build:
	@docker build --pull -t social-studio-api:local .

docker-up:
	@docker compose up --build --wait --remove-orphans

docker-down:
	@docker compose down

clean:
	@rm -rf bin

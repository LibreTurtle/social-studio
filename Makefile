.PHONY: build run test migrate-build migrate-up migrate-down clean

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

clean:
	@rm -rf bin

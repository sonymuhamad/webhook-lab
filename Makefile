.PHONY: run-api run-worker run-receiver migrate migrate-down migrate-status generate test test-integration

run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker

run-receiver:
	go run ./cmd/receiver

migrate:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

migrate-status:
	go run ./cmd/migrate status

generate:
	go generate ./...
	go tool sqlc generate
	go tool wire ./di

test:
	go test ./...

# Needs Docker running: each package with integration tests starts its own
# Postgres container.
test-integration:
	go test -tags integration -count=1 ./...

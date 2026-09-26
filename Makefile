.PHONY: run-api migrate migrate-down migrate-status generate test

run-api:
	go run ./cmd/api

migrate:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

migrate-status:
	go run ./cmd/migrate status

generate:
	go tool wire ./di

test:
	go test ./...

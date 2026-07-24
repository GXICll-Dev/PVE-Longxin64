.PHONY: setup dev-api dev-worker dev-web test lint openapi-lint build compose-up compose-down

setup:
	cd server && go mod download
	cd web && npm install

dev-api:
	cd server && go run ./cmd/api

dev-worker:
	cd server && go run ./cmd/worker

dev-web:
	cd web && npm run dev

test:
	cd server && go test -race ./...
	cd web && npm test

lint:
	cd server && test -z "$$(gofmt -l .)"
	cd server && go vet ./...
	cd web && npm run lint

openapi-lint:
	npx --yes @redocly/cli@2.40.0 lint api/openapi.yaml

build:
	cd server && go build ./cmd/api ./cmd/worker ./cmd/healthcheck
	cd web && npm run build

compose-up:
	cd deploy && docker compose up --build

compose-down:
	cd deploy && docker compose down

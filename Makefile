.PHONY: build up down test lint

build:
	docker compose build

up:
	docker compose up -d

down:
	docker compose down

test:
	cd backend && go test ./...

lint:
	cd frontend && npx tsc -b --noEmit
	cd backend && go vet ./...

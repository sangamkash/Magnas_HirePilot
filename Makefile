.PHONY: help install ui build api test up down compose-build rebuild migrate-up migrate-down migrate-status migrate-create

## Show available Make targets.
help:
	@awk '/^## / {description = substr($$0, 4); next} /^[a-zA-Z0-9_-]+:/ {if (description) printf "\033[36m%-20s\033[0m %s\n", substr($$1, 1, length($$1)-1), description; description = ""}' $(MAKEFILE_LIST)

## Install frontend dependencies from package-lock.json.
install:
	npm ci

## Start the Vite development server.
ui:
	npm run dev

## Create a production frontend build.
build:
	npm run build

## Start the Go backend on the host.
api:
	cd backend && go run ./cmd/api

## Run backend tests, vet, and build checks.
test:
	cd backend && go test ./... && go vet ./... && go build ./...

## Start all Compose services in the background.
up:
	docker compose up -d

## Stop Compose services and preserve named volumes.
down:
	docker compose down

## Build all Compose images.
compose-build:
	docker compose build

## Cleanly reinstall/build the frontend, rebuild all Compose images, and recreate services.
rebuild:
	npm ci
	npm run build
	docker compose build --no-cache
	docker compose up -d --force-recreate

## Apply all pending database migrations.
migrate-up:
	docker compose run --rm backend migrate up

## Roll back the latest migration. Set COUNT=n to roll back n migrations.
migrate-down:
	@if [ -n "$(COUNT)" ]; then \
		docker compose run --rm backend migrate down "$(COUNT)"; \
	else \
		docker compose run --rm backend migrate down; \
	fi

## Show applied and pending database migrations.
migrate-status:
	docker compose run --rm backend migrate status

## Create a paired SQL migration. Usage: make migrate-create name=add_candidate_notes
migrate-create:
	@if [ -z "$(name)" ]; then echo "Usage: make migrate-create name=<migration_name>"; exit 2; fi
	docker compose run --rm backend migrate create "$(name)"

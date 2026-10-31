.PHONY: ui start install build api up down test migrate

## Start the Vite development server.
ui start:
	npm run dev

## Install frontend dependencies.
install:
	npm install

## Create a production build.
build:
	npm run build

## Start the Go backend.
api:
	cd backend && go run ./cmd/api

## Start the full stack from the project root.
up:
	@if [ "$(filter migrate,$(MAKECMDGOALS))" = "migrate" ]; then :; else docker compose up --build -d; fi

## Stop the Compose services without deleting database volumes.
down:
	@if [ "$(filter migrate,$(MAKECMDGOALS))" = "migrate" ]; then :; else docker compose down; fi

## Run the backend migration command from the project root.
## Usage: make migrate up | down | status | create name=<migration_name>
migrate:
	@if [ -z "$(filter-out $@,$(MAKECMDGOALS))" ]; then \
		echo "Usage: make migrate up | down | status | create name=<migration_name>"; \
		exit 2; \
	fi
	@set -e; action="$(word 2,$(MAKECMDGOALS))"; \
	case "$$action" in \
		up|status) docker compose run --rm backend migrate "$$action" ;; \
		down) \
			docker compose run --rm backend migrate down ;; \
		create) \
			name="$(name)"; \
			if [ -z "$$name" ]; then echo "Usage: make migrate create name=<migration_name>"; exit 2; fi; \
			docker compose run --rm backend migrate create "$$name" ;; \
		*) echo "Unknown migration action: $$action"; exit 2 ;; \
	esac

# Allow action words after `migrate` to be parsed as arguments instead of targets.
status create:
	@:

## Run backend tests and static checks.
test:
	cd backend && go test ./... && go vet ./... && go build ./...

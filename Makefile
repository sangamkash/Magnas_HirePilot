.PHONY: ui start install build api migrate-up migrate-down migrate-status test

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

## Apply versioned database migrations using the backend Cobra command.
migrate-up:
	cd backend && go run ./cmd/api migrate up

## Roll back the latest migration using the backend Cobra command.
migrate-down:
	cd backend && go run ./cmd/api migrate down

## Show migration status.
migrate-status:
	cd backend && go run ./cmd/api migrate status

## Run backend tests and static checks.
test:
	cd backend && go test ./... && go vet ./... && go build ./...

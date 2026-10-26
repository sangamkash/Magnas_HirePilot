.PHONY: ui start install build

## Start the Vite development server.
ui start:
	npm run dev

## Install frontend dependencies.
install:
	npm install

## Create a production build.
build:
	npm run build

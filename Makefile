.PHONY: setup up up-all test down logs

-include .env

DB_PORT ?= $(EXTERNAL_DB_PORT)
ifeq ($(DB_PORT),)
    DB_PORT := 5432
endif

setup:
	docker compose up -d redis postgres
	@echo "Waiting for database to be ready..."
	@sleep 3
	@echo "Environment is ready!"

up:
	docker compose up --build

up-all:
	docker compose up -d --build

logs:
	docker compose logs -f app

test:
	DB_DSN="host=127.0.0.1 port=$(DB_PORT) user=postgres password=postgres dbname=taskqueue sslmode=disable" go test -count=1 -v ./...

down:
	docker compose down
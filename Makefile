.PHONY: setup test down

# Автоматически загружаем переменные из .env, если он существует
-include .env

# Если в .env задан EXTERNAL_DB_PORT, берем его. Иначе дефолт — 5432.
DB_PORT ?= $(EXTERNAL_DB_PORT)
ifeq ($(DB_PORT),)
    DB_PORT := 5432
endif

setup:
	docker-compose up -d redis postgres
	@echo "Waiting for database to be ready..."
	@sleep 3
	@echo "Environment is ready!"

test:
	DB_DSN="host=127.0.0.1 port=$(DB_PORT) user=postgres password=postgres dbname=taskqueue sslmode=disable" go test -count=1 -v ./...

down:
	docker-compose down
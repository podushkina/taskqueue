# 🚀 Distributed Task Queue

![Go](https://img.shields.io/badge/Go-1.23-00ADD8?style=flat&logo=go)
![Redis](https://img.shields.io/badge/Redis-7.0-DC382D?style=flat&logo=redis)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-15-4169E1?style=flat&logo=postgresql)
![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?style=flat&logo=docker)
![License](https://img.shields.io/badge/License-MIT-green)

Асинхронная распределённая очередь задач на **Go**, использующая **Redis** как брокер сообщений и **PostgreSQL** для надёжного хранения истории выполнения.

Реализованы паттерны **Worker Pool**, **Exponential Backoff Retry**, **Graceful Shutdown**, автоматические миграции и **Structured Logging** (`slog`).

---

## 📖 Содержание

- [Архитектура](#🏗-архитектура)
- [Ключевые особенности](#✨-ключевые-особенности)
- [Технологический стек](#🛠-технологический-стек)
- [Быстрый старт](#⚡-быстрый-старт)
- [API](#📚-api)
- [Типы задач](#🎯-типы-задач)
- [Retry логика](#🔄-retry-логика)
- [Структура проекта](#📂-структура-проекта)
- [Тесты](#🧪-тесты)
- [Конфигурация](#⚙️-конфигурация)

---
## 🏗 Архитектура

Система состоит из следующих компонентов:

1. **API Server** — принимает HTTP-запросы, валидирует и ставит задачи в Redis через абстрактный интерфейс `TaskEnqueuer`.
2. **Redis Queue** — персистентный брокер сообщений для оперативного управления очередью (хранение тасок, списков и удаление).
3. **Worker Pool** — набор горутин, вычитывающих задачи из Redis, выполняющих логику и логирующих шаги.
4. **PostgreSQL Repository** — изолированный слой данных, куда воркеры через интерфейс `HistoryRepository` сохраняют и обновляют историю выполнения задач.

```
┌──────────────┐       ┌──────────────────────────┐       ┌──────────────┐
│              │       │          Redis           │       │  Worker Pool │
│  Client      │       │                          │       │              │
│  (curl/app)  │──────▶│  taskqueue:pending [list] │──────▶│  Worker 0    │
│              │  HTTP │  taskqueue:task:id {json} │  Pop  │  Worker 1    │
│  POST /tasks │       │                          │◀──────│  Worker 2    │
│  GET /tasks  │       └──────────────────────────┘       │              │
└──────────────┘                                          └──────┬───────┘
                                                                 │
                                                    Save History │
                                                                 ▼
                                                      ┌──────────────────┐
                                                      │    PostgreSQL    │
                                                      │  (task_history)  │
                                                      └──────────────────┘

```
---

## ✨ Ключевые особенности

### Reliability (Надёжность)
- **Retry Policy** — при сбое задача автоматически отправляется на повторную обработку.
- **Exponential Backoff** — интервал ожидания между попытками растёт прогрессивно: `1s → 2s → 4s`.
- **Dead Letter Logic** — по достижении лимита попыток задачи окончательно переводится в статус `failed`.

### Concurrency (Конкурентность)
- Конфигурируемый **Worker Pool** для параллельной безопасной обработки.
- Потокобезопасная архитектура и честный **Graceful Shutdown** через `sync.WaitGroup` и системные сигналы (`SIGINT`, `SIGTERM`). Воркеры гарантированно завершают текущие задачи перед выходом, а бэкенд чисто закрывает соединения с хранилищами.

### Database Migrations (Миграции)
- Интегрирован инструмент **Goose**. Схемы базы данных используют стандартный механизм **Go `embed`**, вшиваются напрямую в бинарный файл при сборке и накатываются автоматически при запуске приложения.

### Observability (Наблюдаемость)
- **Structured Logging** в формате JSON через стандартный пакет `log/slog`. Каждый контекстный лог содержит метаданные `task_id` и `worker_id`.

---

## 🛠 Технологический стек

| Категория | Технология |
|-----------|-----------|
| Язык | Go 1.23+ |
| Брокер очередей | Redis 7 (go-redis/v9) |
| База данных | PostgreSQL 15 |
| Утилита миграций | goose v3 (с поддержкой `go:embed`) |
| HTTP роутер | chi v5 |
| Конфигурация | Переменные окружения (12-Factor App) |
| Тесты | `testing` + `testify` + интеграционные тесты для БД |
| Контейнеризация | Docker, Docker Compose (с healthchecks) |
| Логирование | `log/slog` (structured JSON) |

---

## ⚡ Быстрый старт

### Docker Compose 

```bash
git clone [https://github.com/podushkina/taskqueue.git](https://github.com/podushkina/taskqueue.git)
cd taskqueue
docker-compose up --build
```
Инфраструктура поднимется, миграции выполнятся сами, а сервер запустится на порту `:8080`.

### Локально через Makefile

```bash
# 1. Поднять инфраструктуру (Redis + PostgreSQL)
make setup

# 2. Настроить окружение
cp .env.example .env  # (отредактируйте .env при необходимости)

# 3. Запустить приложение
go run cmd/server/main.go
```

---

## 📚 API

### Создать задачу

**`POST /tasks`**

```bash
curl -X POST http://localhost:8080/tasks \
  -H "Content-Type: application/json" \
  -d '{"type": "echo", "payload": "Hello World"}'
```

Ответ (`201 Created`):
```json
{
  "id": "5335d502-a2c7-4abd-9496-c64692b43869",
  "type": "echo",
  "payload": "Hello World",
  "status": "pending",
  "retries": 0,
  "max_retry": 3,
  "created_at": "2026-02-01T10:53:32Z",
  "updated_at": "2026-02-01T10:53:32Z"
}
```

### Получить статус задачи

**`GET /tasks/{id}`**

```bash
curl http://localhost:8080/tasks/5335d502-a2c7-4abd-9496-c64692b43869
```

Ответ (`200 OK`):
```json
{
  "id": "5335d502-a2c7-4abd-9496-c64692b43869",
  "type": "echo",
  "status": "completed",
  "result": "echo: Hello World",
  "retries": 0,
  "max_retry": 3,
  "created_at": "2026-02-01T10:53:32Z",
  "updated_at": "2026-02-01T10:53:33Z"
}
```

### Список всех задач

**`GET /tasks`**

```bash
curl http://localhost:8080/tasks
```

### Удалить задачу

**`DELETE /tasks/{id}`**

```bash
curl -X DELETE http://localhost:8080/tasks/5335d502-a2c7-4abd-9496-c64692b43869
```

Ответ: `204 No Content`

### Health Check

**`GET /health`**

```bash
curl http://localhost:8080/health
# {"status": "ok"}
```

---

## 🎯 Типы задач

| Тип | Описание | Пример Payload | Пример Result |
|-----|----------|---------------|--------------|
| `echo` | Возвращает payload | `"Hello"` | `"echo: Hello"` |
| `reverse` | Переворачивает строку | `"Hello"` | `"olleH"` |
| `sum` | Суммирует JSON-массив чисел | `"[1, 2, 5]"` | `"8.00"` |
| `slow` | Имитация долгой операции (5 сек) | любой | `"completed after 5 seconds"` |
| `flaky` | Случайно падает (демо retry) | любой | `"flaky: ..."` или ошибка |

---

## 🔄 Retry логика

При ошибке обработки задача автоматически возвращается в очередь с экспоненциальным backoff:

```
Попытка 1: ошибка  →  ждём 1s  →  retry
Попытка 2: ошибка  →  ждём 2s  →  retry
Попытка 3: ошибка  →  ждём 4s  →  retry
Попытка 4: ошибка  →  статус "failed" (max retries exceeded)
```

Жизненный цикл задачи:

```
                  ┌───────────────────────────────┐
                  │                               │
                  ▼                               │
┌─────────┐   ┌────────────┐   ┌───────────┐     │
│ pending │──▶│ processing │──▶│ completed │     │
└─────────┘   └────────────┘   └───────────┘     │
                  │                               │
                  │ error + retries < max         │
                  │                               │
                  └───── backoff ── retry ─────────┘
                  │
                  │ error + retries >= max
                  ▼
              ┌──────────┐
              │  failed  │
              └──────────┘
```

---

## 📂 Структура проекта

```
taskqueue/
├── cmd/
│   └── server/
│       └── main.go           # Точка входа, DI, Graceful Shutdown
├── internal/
│   ├── api/
│   │   ├── handler.go        # HTTP хендлеры
│   │   ├── handler_test.go
│   │   └── router.go         # Роутер chi и middleware
│   ├── config/
│   │   └── config.go         # Парсинг переменных окружения
│   ├── model/
│   │   └── task.go           # Общие структуры данных (Task)
│   ├── repository/
│   │   ├── postgres.go       # Хранение истории в PostgreSQL
│   │   ├── postgres_test.go  # Интеграционный тест БД
│   │   ├── redis.go          # Очередь задач в Redis
│   │   └── redis_test.go     # Интеграционный тест очередей
│   └── worker/
│       ├── pool.go           # Оркестратор Worker Pool
│       ├── pool_test.go      # Тест конкурентности и отмены контекстов
│       └── jobs.go           # Логика воркеров (Echo, Sum...)
├── migrations/
│   ├── 00001_init_tasks.sql  # SQL-миграция для создания таблиц
│   └── migrations.go         # Логика запуска миграций через goose
├── docker-compose.yml
├── Dockerfile
├── Makefile
├── .gitignore
├── go.mod
└── go.sum
```

---

## 🧪 Тесты

Вручную:

```bash
# Все тесты
go test ./... -v

# С покрытием
go test ./... -cover

# С проверкой на гонки данных
go test ./... -race
```

Для запуска тестов с автоматической подстановкой нужных портов и переменных окружения (используя инфраструктуру в Docker):

```bash
make test
```
---

## ⚙️ Конфигурация

Настраивается через переменные окружения или файл `.env` (в корне проекта). Если приложение запускается через Docker Compose, значения берутся из секции `environment:` в `docker-compose.yml`.

### Переменные окружения

| Переменная | Описание | Значение по умолчанию |
|-----------|----------|-----------------------|
| `SERVER_PORT` | Порт API сервера | `8080` |
| `DB_DSN` | Строка подключения к PostgreSQL | `host=localhost user=postgres password=postgres dbname=taskqueue sslmode=disable` |
| `REDIS_ADDR` | Адрес Redis сервера | `localhost:6379` |
| `REDIS_PASSWORD` | Пароль Redis | _(пусто)_ |
| `REDIS_DB` | Номер базы Redis (0–15) | `0` |
| `WORKER_COUNT` | Количество воркеров в пуле | `3` |
| `SHUTDOWN_TIMEOUT` | Тайм-аут на Graceful Shutdown | `10s` |

---

### Примечание по DSN для Docker
Если вы запускаете Go-приложение локально, а базы — в Docker (`make setup`), используйте дефолтные значения (на `localhost`).
Если вы запускаете всё через `docker-compose up`, DSN должен использовать имена сервисов (например, `host=postgres`). В дефолтном `docker-compose.yml` это уже настроено.

---

## 📄 Лицензия

Этот проект распространяется под лицензией [MIT](LICENSE).



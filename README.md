# Distributed Task Queue

![Go](https://img.shields.io/badge/Go-1.23-00ADD8?style=flat&logo=go)
![Redis](https://img.shields.io/badge/Redis-7.0-DC382D?style=flat&logo=redis)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-15-4169E1?style=flat&logo=postgresql)
![Prometheus](https://img.shields.io/badge/Prometheus-Monitoring-E6522C?style=flat&logo=prometheus)
![Grafana](https://img.shields.io/badge/Grafana-Dashboard-F46800?style=flat&logo=grafana)
![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?style=flat&logo=docker)
![License](https://img.shields.io/badge/License-MIT-green)

Асинхронная распределенная очередь задач на Go с брокером на Redis и хранилищем истории в PostgreSQL.

Проект включает пул воркеров, неблокирующий экспоненциальный retry, сохранение истории и аналитику в PostgreSQL, сбор RED-метрик для Prometheus и преднастроенный дашборд в Grafana.

---

## Содержание

- [Архитектура](#архитектура)
- [Реализованные механизмы](#реализованные-механизмы)
- [Мониторинг и Observability](#мониторинг-и-observability)
- [Технологический стек](#технологический-стек)
- [Запуск](#запуск)
- [API](#api)
- [Типы задач](#типы-задач)
- [Жизненный цикл задачи](#жизненный-цикл-задачи)
- [Структура проекта](#структура-проекта)
- [Тесты](#тесты)
- [Конфигурация](#конфигурация)
- [Лицензия](#лицензия)

---

## Архитектура

```
┌──────────────┐        ┌──────────────────────────┐        ┌──────────────┐
│              │        │          Redis           │        │  Worker Pool │
│   Client     │        │                          │        │              │
│  (curl/app)  │───────▶│  taskqueue:pending [list]│───────▶│   Worker 0   │
│              │  HTTP  │  taskqueue:task:id {json}│  Pop   │   Worker 1   │
│ POST /tasks  │        │                          │◀───────│   Worker 2   │
│ GET /tasks   │        └──────────────────────────┘        │              │
└──────────────┘                                            └──────┬───────┘
       │                                                           │
       │ GET /analytics                               Save History │
       │ GET /metrics                                              │
       ▼                                                           ▼
┌──────────────────┐                                        ┌──────────────┐
│ Prometheus /     │◀───────────────────────────────────────│  PostgreSQL  │
│ Grafana          │            Aggregate Analytics         │ task_history │
└──────────────────┘                                        └──────────────┘
```

1. **API Server (`chi`)**: принимает запросы, ставит задачи в Redis, отдает историю/аналитику и экспортирует метрики на `/metrics`.
2. **Redis Queue**: хранит списки задач (`LPUSH`, `RPOP`) и их текущие состояния.
3. **Worker Pool**: набор горутин, вычитывающих задачи из брокера. Включает перехват паник (`recover`) и Graceful Shutdown.
4. **PostgreSQL Repository**: сохраняет историю выполненных задач. Схема содержит составной B-Tree индекс для агрегации аналитики.

---

## Реализованные механизмы

### Обработка ошибок и надежность
- **Panic Recovery**: если обработчик задачи падает с паникой, воркер перехватывает ее через `recover()`, пул продолжает работу, а задача получает статус `failed`.
- **Non-blocking Retries**: пауза перед повторной попыткой реализована через `time.NewTimer` с `select`, что дает возможность мгновенно остановить воркер при отмене контекста.
- **Exponential Backoff**: интервал ожидания между попытками растет: `1s → 2s → 4s`. При исчерпании лимита (`max_retry`) задача переходит в статус `failed`.

### Работа с базой данных
- **Composite B-Tree Index**: индекс `(status, created_at DESC)` в таблице `task_history` исключает Full Table Scan при выборке истории.
- **Single-Query Aggregation**: ручка `/analytics` рассчитывает статистику по статусам и среднюю длительность задач за один агрегационный SQL-запрос.

### Метрики и логирование
- **RED Metrics**: HTTP-middleware фиксирует частоту запросов (Rate), ошибки (Errors) и гистограммы длительности обработки задач (Duration). Воркеры экспортируют глубину очереди и счетчики ретраев.
- **Structured Logging**: логирование в формате JSON через `log/slog` с полями `task_id` и `worker_id`.

---

## Мониторинг и Observability

Стек мониторинга разворачивается в `docker-compose.yml`:
* **Prometheus:** `http://localhost:9090`
* **Grafana:** `http://localhost:3000` (анонимный доступ включен)
* **Метрики приложения:** `http://localhost:8080/metrics`

![Grafana Dashboard](docs/images/grafana-dashboard.png)

---

## Технологический стек

| Категория | Технология |
|---|---|
| Язык | Go 1.23+ |
| Брокер очередей | Redis 7 (`go-redis/v9`) |
| База данных | PostgreSQL 15 |
| Миграции | Goose v3 (`go:embed`) |
| Мониторинг | Prometheus + Grafana |
| HTTP роутер | Chi v5 |
| Конфигурация | Переменные окружения |
| Тесты | `testing`, `testify`, интеграционные тесты |
| Контейнеризация | Docker, Docker Compose |
| Логирование | `log/slog` |

---

## Запуск

### Запуск через Docker Compose

```bash
git clone [https://github.com/podushkina/taskqueue.git](https://github.com/podushkina/taskqueue.git)
cd taskqueue
make up
```

После старта доступны:
* API: `http://localhost:8080`
* Grafana: `http://localhost:3000`
* Prometheus: `http://localhost:9090`

### Локальный запуск

```bash
# 1. Запустить базы и мониторинг в фоне
docker compose up -d redis postgres prometheus grafana

# 2. Запустить сервер
go run ./cmd/server
```

---

## API

### 1. Создать задачу

**`POST /tasks`**

```bash
curl -X POST http://localhost:8080/tasks \
  -H "Content-Type: application/json" \
  -d '{"type": "sum", "payload": "10,20,30"}'
```

Ответ (`201 Created`):
```json
{
  "id": "ebe2fdf7-09b4-4cae-a994-1a659757e739",
  "type": "sum",
  "payload": "10,20,30",
  "status": "pending",
  "retries": 0,
  "max_retry": 3,
  "created_at": "2026-08-14T18:39:13.490Z",
  "updated_at": "2026-08-14T18:39:13.490Z"
}
```

### 2. Получить статус задачи

**`GET /tasks/{id}`**

```bash
curl http://localhost:8080/tasks/ebe2fdf7-09b4-4cae-a994-1a659757e739
```

Ответ (`200 OK`):
```json
{
  "id": "ebe2fdf7-09b4-4cae-a994-1a659757e739",
  "type": "sum",
  "status": "completed",
  "result": "60.00",
  "retries": 0,
  "max_retry": 3,
  "created_at": "2026-08-14T18:39:13.490Z",
  "updated_at": "2026-08-14T18:39:13.502Z"
}
```

### 3. Операционная аналитика

**`GET /analytics`**  
Параметры фильтрации `from` и `to` опциональны (формат RFC3339).

```bash
curl "http://localhost:8080/analytics?from=2026-08-14T00:00:00Z&to=2026-08-14T23:59:59Z"
```

Ответ (`200 OK`):
```json
{
  "total_tasks": 172,
  "status_counts": {
    "completed": 67,
    "failed": 105,
    "pending": 0,
    "processing": 0
  },
  "avg_duration_seconds": 3.5052
}
```

### 4. Список задач в очереди

**`GET /tasks`**

```bash
curl http://localhost:8080/tasks
```

### 5. Удалить задачу

**`DELETE /tasks/{id}`**

```bash
curl -X DELETE http://localhost:8080/tasks/ebe2fdf7-09b4-4cae-a994-1a659757e739
```

Ответ: `204 No Content`

### 6. Health Check

**`GET /health`**

```bash
curl http://localhost:8080/health
# {"status": "ok"}
```

---

## Типы задач

| Тип | Описание | Пример Payload | Результат |
|---|---|---|---|
| `echo` | Возвращает переданный payload | `"Hello"` | `"echo: Hello"` |
| `reverse` | Переворачивает строку | `"golang"` | `"gnalog"` |
| `sum` | Суммирует числа через запятую | `"10,20,30"` | `"60.00"` |
| `slow` | Имитация длительной операции (5 сек) | любой | `"completed after 5 seconds"` |
| `flaky` | Имитация нестабильной работы для тестов | любой | Результат или ошибка |

---

## Жизненный цикл задачи

```
                  ┌───────────────────────────────┐
                  │                               │
                  ▼                               │
┌─────────┐   ┌────────────┐   ┌───────────┐      │
│ pending │──▶│ processing │──▶│ completed │      │
└─────────┘   └────────────┘   └───────────┘      │
                  │                               │
                  │ error/panic + retries < max   │
                  │ (non-blocking time.NewTimer)  │
                  └───── backoff ── retry ────────┘
                  │
                  │ error + retries >= max
                  ▼
              ┌──────────┐
              │  failed  │ (DLQ)
              └──────────┘
```

---

## Структура проекта

```
taskqueue/
├── cmd/
│   └── server/
│       └── main.go                 # Точка входа, DI, Graceful Shutdown
├── grafana/
│   └── provisioning/
│       ├── dashboards/             # Конфигурация дашборда
│       └── datasources/            # Подключение Prometheus
├── internal/
│   ├── api/
│   │   ├── handler.go              # HTTP-хендлеры
│   │   ├── handler_test.go         # Unit-тесты ручек
│   │   ├── middleware.go           # Сбор RED-метрик
│   │   └── router.go               # Роутинг и эндпоинт /metrics
│   ├── config/
│   │   └── config.go               # Чтение конфигурации
│   ├── metrics/
│   │   └── prometheus.go           # Prometheus метрики
│   ├── model/
│   │   ├── analytics.go            # Модель аналитики
│   │   └── task.go                 # Модель Task
│   ├── repository/
│   │   ├── postgres.go             # Слой работы с PostgreSQL
│   │   ├── postgres_test.go        # Интеграционные тесты БД
│   │   ├── redis.go                # Слой работы с Redis
│   │   └── redis_test.go           # Интеграционные тесты Redis
│   └── worker/
│       ├── jobs.go                 # Обработчики типов задач
│       ├── pool.go                 # Worker Pool, Panic Recovery, Backoff
│       └── pool_test.go            # Тесты пула воркеров
├── migrations/
│   ├── 00001_init_tasks.sql        # Схема таблицы task_history
│   ├── 00002_add_status_index.sql  # Составной индекс
│   └── migrations.go               # Запуск Goose миграций (go:embed)
├── docker-compose.yml
├── Dockerfile
├── Makefile
└── prometheus.yml
```

---

## Тесты

Запуск тестов:

```bash
make test
```

Запуск с race detector:

```bash
go test -race -v ./...
```

---

## Конфигурация

| Переменная | Описание | По умолчанию |
|---|---|---|
| `SERVER_PORT` | Порт HTTP API | `8080` |
| `DB_DSN` | Строка подключения к PostgreSQL | `host=postgres user=postgres password=postgres dbname=taskqueue sslmode=disable` |
| `REDIS_ADDR` | Адрес Redis | `redis:6379` |
| `REDIS_PASSWORD` | Пароль Redis | _(пусто)_ |
| `REDIS_DB` | База данных Redis | `0` |
| `WORKER_COUNT` | Количество воркеров в пуле | `3` |
| `SHUTDOWN_TIMEOUT` | Таймаут Graceful Shutdown | `10s` |

---

## Лицензия

Проект распространяется под лицензией [MIT](LICENSE).

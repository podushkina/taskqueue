# TaskQueue

Distributed task queue with Go and Redis.

## Features

- Worker pool with configurable concurrency
- Graceful shutdown
- REST API
- Redis backend
- Docker ready

## Quick Start
```bash
docker-compose up --build
```

## API

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | /tasks | Create task |
| GET | /tasks | List tasks |
| GET | /tasks/{id} | Get task |
| DELETE | /tasks/{id} | Delete task |

## Example
```bash
# Create task
curl -X POST http://localhost:8080/tasks \
  -H "Content-Type: application/json" \
  -d '{"type": "echo", "payload": "Hello"}'

# Get result
curl http://localhost:8080/tasks/{id}
```

## Task Types

- `echo` — returns payload
- `reverse` — reverses string
- `sum` — sums JSON array of numbers
- `slow` — 5 second delay (demo)
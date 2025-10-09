# v1
# file: README.md
NRG CHAMP Gamification Service — HTTP skeleton for the global leaderboard

This module provides the initial process skeleton for the future
leaderboard API. The implementation focuses on configuration loading,
structured logging, health endpoints, and lifecycle management. Actual
leaderboard computations will land in later iterations.

## Layout

```
services/gamification/
├── cmd/gamification/        # Entry point wiring config + app lifecycle
├── internal/app/            # Logger fan-out and server orchestration
├── internal/config/         # Defaults, properties, and env parsing
├── internal/http/           # Router, health handlers, middleware
└── gamification.properties  # Sample properties file with defaults
```

## Configuration

Settings are layered as defaults, the optional properties file, then
environment variables. The following keys are currently recognised:

| Key | Description | Default |
| --- | ----------- | ------- |
| `GAMIFICATION_PROPERTIES_PATH` | Path to the properties file. | `gamification.properties` |
| `listen_address` | Properties key overriding the HTTP bind address. | `:8086` |
| `GAMIFICATION_LISTEN_ADDRESS` | Environment override for the bind address. | `:8086` |
| `log_path` | Properties key selecting the log file location. | `logs/gamification.log` |
| `GAMIFICATION_LOG_PATH` | Environment override for the log file. | `logs/gamification.log` |
| `http_read_timeout_ms` | Milliseconds for reading request headers/body. | `5000` |
| `GAMIFICATION_HTTP_READ_TIMEOUT_MS` | Environment override for the same timeout. | `5000` |
| `http_write_timeout_ms` | Milliseconds allowed for sending responses. | `10000` |
| `GAMIFICATION_HTTP_WRITE_TIMEOUT_MS` | Environment override for the same timeout. | `10000` |
| `shutdown_timeout_ms` | Milliseconds granted for graceful shutdown. | `5000` |
| `GAMIFICATION_SHUTDOWN_TIMEOUT_MS` | Environment override for the same timeout. | `5000` |

When running via Docker the properties file is copied next to the binary
and referenced through the `GAMIFICATION_PROPERTIES_PATH` variable.

## Health Endpoints

The HTTP server currently exposes:

- `GET /health` and `GET /health/live` returning `200 OK` while the
  process is alive.
- `GET /health/ready` returning `503 Service Unavailable` until the
  router is fully initialised. Once ready, the endpoint responds `200 OK`.

## Running Locally

From the repository root:

```bash
mkdir -p bin
go build -o ./bin/gamification ./services/gamification/cmd/gamification
GAMIFICATION_LOG_PATH=./logs/gamification.log ./bin/gamification
```

The service logs to both stdout and the configured log file. Ensure the
log directory exists or allow the service to create it with the default
permissions (0755).

## Next Steps

Future tasks will hook the router to real leaderboard storage, connect
Kafka consumers, and expose read-only leaderboard APIs consistent with
`docs/project_documentation.md` and `docs/ragionamenti.md`.

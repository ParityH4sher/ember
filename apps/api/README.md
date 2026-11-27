# Ember API module

This directory is the API's standalone Go module (`go.mod`). Build and test it
from here:

```sh
go test ./...
go build .
```

## Layout

- `main.go`, `accesslog.go`: process startup, HTTP server lifecycle, and access logging.
- `internal/httpapi/`: HTTP routing, request validation, plugin/project handlers.
- `internal/plugins/`: plugin contract types.
- `internal/plugins/weather/`: versioned Weather plugin adapter for Open-Meteo.
- `internal/projects/`: disk-backed project registration, Markdown vaults and
  chat transcripts (`chats/<id>.json`); an in-memory index lists active projects.
- `internal/agents/`: project chat's agents file (`EMBER_AGENTS_CONFIG_FILE`),
  the `ember.chat-turn/v1` envelope and the sandbox router client
  ([project chat](../../docs/project-chat.md)).
- `internal/aicatalog/`: model providers, models and agents that admins add in
  the web app (`/v1/admin/ai`), saved to `EMBER_AI_CATALOG_FILE` (mode 0600; it
  holds provider keys). Without the variable the catalog is kept in memory.
- `internal/projectfiles/`: filesystem storage for one project’s contents, independent of HTTP project records.

The API module is compiled as one service, while the `internal/` packages keep
HTTP, plugin adapters, and project storage separated by responsibility. Plugin
registration is compile-time; there is no dynamic plugin loader.

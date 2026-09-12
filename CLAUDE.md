# Kilat Pet Delivery - service-notification

Multi-channel delivery driven by Kafka events: email, push and SMS, with per-user channel preferences.
Jira project **KPD** - GitHub `Kilat-Pet-Delivery/service-notification` - stack **Go 1.24 - Gin - GORM - PostgreSQL - Kafka**. Global rules live in `~/.claude/`;
this file only adds what is specific here.

## Orient here first

- `.claude/memory/project_state.md` - **resume here** (`/continue` reads it, `/recap` rewrites it).
- `README.md` - how to run it. `CHANGELOG.md` - what changed.
- The workspace map: `~/Documents/kilat-pet-delivery/CLAUDE.md`.

## Commands

| Task | Command |
|---|---|
| install | `go mod download` |
| run | `go run ./cmd/server` (copy `.env.example` to `.env` first) |
| test | `go test ./...` |
| integration tests | `go test -tags integration ./...` - needs Docker (testcontainers) |
| lint | `gofmt -l . && go vet ./...` |
| build | `go build ./...` |
| migrate | `go run ./cmd/migrate` - applies `migrations/` and exits |

Needs the dev-infra stack: Postgres database `kilat_notification`, Kafka on `localhost:9092` -> `cd ~/Documents/dev-infra; ./dev.ps1 up kilat`.

## Conventions that differ from the global rules

- **Ticket branches and PRs** - company repo, never commit on `main` (`branch-guard` enforces it).
- **One migration path.** `migrations/` owns the schema in every environment including development, and `cmd/server` applies it at startup. There is deliberately no GORM AutoMigrate branch - that is what let six services drift (KPD-56 through KPD-61).
- Protected paths (never edited in place, see `.claude/protected-paths.txt`): `migrations/*.sql`.

## Testing

`go test ./...` - 16 passing. One of the four test files is behind the `integration` tag.

## Where things are

- `cmd/server` - `cmd/migrate` - `internal/events/` the Kafka consumers that trigger sends - `templates/` message templates - `internal/domain/notification`

## Worth knowing

- Locally, email goes to Mailpit from the dev-infra stack (localhost:1025, UI on 8025). FCM and Twilio credentials are left empty in .env.example, so those channels stay off until you fill them.

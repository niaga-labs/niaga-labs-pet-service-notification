# service-notification

Multi-channel notification delivery for Kilat Pet Delivery, driven by Kafka events. Go 1.24 · Gin ·
GORM · PostgreSQL · Kafka. Listens on port **8006**.

## Running the Service

```bash
# Install dependencies
go mod download

# Point at the shared dev-infra stack (cd ~/Documents/dev-infra; ./dev.ps1 up kilat)
export DB_HOST=localhost DB_PORT=5432 DB_USER=kilat DB_PASSWORD=kilat_secret
export DB_NAME=kilat_notification DB_SSL_MODE=disable
export KAFKA_BROKERS=localhost:9092

# Apply the SQL migrations -- run from the repository root, the migration
# source is resolved relative to the working directory
go run ./cmd/migrate

# Start the service
go run ./cmd/server
```

### Two migration modes

`cmd/migrate` applies the golang-migrate files in `migrations/` and is the source of truth for the
schema. The server additionally auto-migrates the GORM models when `APP_ENV=development`.

Unlike most of the other services, this one's two paths currently agree: `notifications` and
`notification_preferences` are both covered by SQL migrations. (Compare KPD-56 · KPD-57 · KPD-58 ·
KPD-59 · KPD-60 · KPD-61, where they do not.)

## Database schema

| Table | Purpose |
|---|---|
| `notifications` | one row per delivered or pending notification |
| `notification_preferences` | per-user channel opt-ins |

Migrations: `001_create_notifications`, `002_create_preferences`, `003_add_notification_read_at`.

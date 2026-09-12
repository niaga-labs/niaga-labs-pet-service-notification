# Changelog

All notable changes to service-notification are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `cmd/migrate`: standalone command that applies the golang-migrate files in
  `migrations/` and exits. There was previously no way to apply the SQL schema
  without booting the service outside `APP_ENV=development`. (KPD-4)
- `CHANGELOG.md`: this file. Partially advances KPD-52.

### Changed

- README: the repository had none. Documents the run and migrate commands against
  the shared dev-infra stack, and the schema.

- `cmd/server`: the development-only GORM `AutoMigrate` branch is gone. This
  service had no drift to fix, but keeping that branch is the mechanism that
  produced KPD-56 through KPD-61 elsewhere. The migrations now own the schema in
  all environments and development still gets it automatically at startup.

### Notes

- Migrations applied to `kilat_notification` on the shared dev-infra stack as part of KPD-4.

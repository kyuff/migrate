# migrate

[![Build Status](https://github.com/kyuff/migrate/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/kyuff/migrate/actions/workflows/go.yml)
[![Report Card](https://goreportcard.com/badge/github.com/kyuff/migrate)](https://goreportcard.com/report/github.com/kyuff/migrate/)
[![Go Reference](https://pkg.go.dev/badge/github.com/kyuff/migrate.svg)](https://pkg.go.dev/github.com/kyuff/migrate)
[![codecov](https://codecov.io/gh/kyuff/migrate/graph/badge.svg)](https://codecov.io/gh/kyuff/migrate)

Go library for applying SQL migrations to isolated per-feature Postgres schemas.

Each feature gets its own Postgres schema, keeping migrations and tables cleanly separated. Migration files are identified by a numeric prefix (`000_description.sql`), applied in order, and tracked with a checksum to detect tampering.

## Installation

```sh
go get github.com/kyuff/migrate
```

Requires Go 1.25 or later. Depends on `github.com/jackc/pgx/v5`.

## Usage

```go
pool, _ := pgxpool.New(ctx, os.Getenv("POSTGRES_URL"))

m := migrate.NewMigrator(pool, migrate.WithLogger(slog.Default()))

//go:embed ddl/*.sql
var ddl embed.FS

err := m.Migrate(ctx, "orders", ddl)
```

Migration files must be placed inside the embedded FS and match the pattern `NNN_description.sql` where `NNN` is a zero-padded integer:

```
ddl/
  001_create_orders.sql
  002_add_status_column.sql
```

Each feature name maps to a Postgres schema. The migrator creates the schema and a `_migrations` tracking table on first run. Subsequent runs skip already-applied migrations and verify their checksums.

## Options

```go
migrate.WithLogger(logger *slog.Logger)
```

## License

MIT

package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var migrationFilePattern = regexp.MustCompile(`^\d{3}_[a-z][a-z0-9_]*\.sql$`)

// Migrator applies SQL migration files to isolated per-feature Postgres schemas.
type Migrator struct {
	pool *pgxpool.Pool
	cfg  *Config
}

// NewMigrator creates a Migrator backed by the given connection pool.
func NewMigrator(pool *pgxpool.Pool, opts ...Option) *Migrator {
	return &Migrator{
		pool: pool,
		cfg:  applyOptions(defaultConfig(), opts...),
	}
}

// Migrate applies all pending migrations from ddl to a Postgres schema named
// after featureName. Migration files must match the pattern 000_description.sql
// and are executed in lexicographic order. Already-applied migrations are
// skipped. The FS is walked to find migration files at any depth.
func (m *Migrator) Migrate(ctx context.Context, featureName string, ddl fs.FS) error {
	if err := validateFeatureName(featureName); err != nil {
		m.cfg.Logger.ErrorContext(ctx, fmt.Sprintf("[migrate] Validated feature name %q failed: %s", featureName, err), "feature", featureName, "error", err)
		return err
	}

	migrations, err := collectMigrations(ddl)
	if err != nil {
		m.cfg.Logger.ErrorContext(ctx, fmt.Sprintf("[migrate] Collected migration files for %q failed: %s", featureName, err), "feature", featureName, "error", err)
		return fmt.Errorf("migrate %s: %w", featureName, err)
	}

	if err := m.ensureSchema(ctx, featureName); err != nil {
		m.cfg.Logger.ErrorContext(ctx, fmt.Sprintf("[migrate] Created schema for %q failed: %s", featureName, err), "feature", featureName, "error", err)
		return fmt.Errorf("migrate %s: %w", featureName, err)
	}

	applied, err := m.appliedMigrations(ctx, featureName)
	if err != nil {
		m.cfg.Logger.ErrorContext(ctx, fmt.Sprintf("[migrate] Queried applied migrations for %q failed: %s", featureName, err), "feature", featureName, "error", err)
		return fmt.Errorf("migrate %s: %w", featureName, err)
	}

	var newlyApplied int
	for _, mig := range migrations {
		record, ok := applied[mig.id]
		if ok {
			content, err := fs.ReadFile(ddl, mig.path)
			if err != nil {
				m.cfg.Logger.ErrorContext(ctx, fmt.Sprintf("[migrate] Read migration file %q for %q failed: %s", mig.filename, featureName, err), "feature", featureName, "file", mig.filename, "error", err)
				return fmt.Errorf("migrate %s: reading %s: %w", featureName, mig.filename, err)
			}
			got := checksum(content)
			if got != record.checksum {
				m.cfg.Logger.ErrorContext(ctx, fmt.Sprintf("[migrate] Verified checksum for %q in %q failed: applied %s, current %s", mig.filename, featureName, record.checksum, got), "feature", featureName, "file", mig.filename, "applied_checksum", record.checksum, "current_checksum", got)
				return fmt.Errorf("migrate %s: checksum mismatch for %s (applied: %s, current: %s)", featureName, mig.filename, record.checksum, got)
			}
			continue
		}
		err := m.applyMigration(ctx, featureName, mig, ddl)
		if err != nil {
			m.cfg.Logger.ErrorContext(ctx, fmt.Sprintf("[migrate] Applied migration %q for %q failed: %s", mig.filename, featureName, err), "feature", featureName, "file", mig.filename, "error", err)
			return fmt.Errorf("migrate %s: %w", featureName, err)
		}
		m.cfg.Logger.InfoContext(ctx, fmt.Sprintf("[migrate] Applied migration %q for %q", mig.filename, featureName), "feature", featureName, "file", mig.filename)
		newlyApplied++
	}

	m.cfg.Logger.InfoContext(ctx, fmt.Sprintf("[migrate] Completed %q: %d applied, %d total", featureName, newlyApplied, len(applied)+newlyApplied), "feature", featureName, "applied", newlyApplied, "total", len(applied)+newlyApplied)
	return nil
}

var featureNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func validateFeatureName(name string) error {
	if !featureNamePattern.MatchString(name) {
		return fmt.Errorf("invalid feature name %q: must match %s", name, featureNamePattern.String())
	}
	return nil
}

type migration struct {
	id       int
	filename string
	path     string
}

func collectMigrations(ddl fs.FS) ([]migration, error) {
	seen := make(map[int]string)
	var migrations []migration

	err := fs.WalkDir(ddl, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !migrationFilePattern.MatchString(name) {
			return fmt.Errorf("invalid migration filename %q: must match %s", name, migrationFilePattern.String())
		}
		id, _ := strconv.Atoi(name[:3])
		if prev, ok := seen[id]; ok {
			return fmt.Errorf("duplicate migration id %03d: %s and %s", id, prev, name)
		}
		seen[id] = name
		migrations = append(migrations, migration{id: id, filename: name, path: path})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading migrations: %w", err)
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].id < migrations[j].id
	})
	return migrations, nil
}

func (m *Migrator) ensureSchema(ctx context.Context, featureName string) error {
	sql := fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, pgIdent(featureName))
	if _, err := m.pool.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating schema: %w", err)
	}

	sql = fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s._migrations (
			id INT PRIMARY KEY,
			filename TEXT NOT NULL,
			checksum TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`, pgIdent(featureName))
	if _, err := m.pool.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating migrations table: %w", err)
	}
	return nil
}

type migrationRecord struct {
	checksum string
}

func (m *Migrator) appliedMigrations(ctx context.Context, featureName string) (map[int]migrationRecord, error) {
	sql := fmt.Sprintf(`SELECT id, checksum FROM %s._migrations`, pgIdent(featureName))
	rows, err := m.pool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("querying applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]migrationRecord)
	for rows.Next() {
		var (
			id       int
			checksum string
		)
		if err := rows.Scan(&id, &checksum); err != nil {
			return nil, fmt.Errorf("scanning migration row: %w", err)
		}
		applied[id] = migrationRecord{checksum: checksum}
	}
	return applied, rows.Err()
}

func checksum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func (m *Migrator) applyMigration(ctx context.Context, featureName string, mig migration, ddl fs.FS) error {
	content, err := fs.ReadFile(ddl, mig.path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", mig.filename, err)
	}

	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx for %s: %w", mig.filename, err)
	}
	defer tx.Rollback(ctx)

	// Set search_path so unqualified table names resolve to the feature schema.
	if _, err := tx.Exec(ctx, fmt.Sprintf(`SET LOCAL search_path TO %s`, pgIdent(featureName))); err != nil {
		return fmt.Errorf("setting search_path for %s: %w", mig.filename, err)
	}

	if _, err := tx.Exec(ctx, string(content)); err != nil {
		return fmt.Errorf("executing %s: %w", mig.filename, err)
	}

	recordSQL := fmt.Sprintf(
		`INSERT INTO %s._migrations (id, filename, checksum) VALUES ($1, $2, $3)`, pgIdent(featureName))
	if _, err := tx.Exec(ctx, recordSQL, mig.id, mig.filename, checksum(content)); err != nil {
		return fmt.Errorf("recording %s: %w", mig.filename, err)
	}

	return tx.Commit(ctx)
}

// pgIdent quotes a SQL identifier to prevent injection.
func pgIdent(name string) string {
	return pgx.Identifier{name}.Sanitize()
}

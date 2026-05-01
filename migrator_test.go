package migrate_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/kyuff/migrate/internal/assert"
	"github.com/kyuff/migrate/internal/postgres"

	"github.com/kyuff/migrate"
)

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

// testSchema returns a unique, valid Postgres schema name derived from the
// test name and a short timestamp. The schema is left in the database after
// the test so developers can inspect it.
func testSchema(t *testing.T) string {
	t.Helper()
	name := strings.ToLower(t.Name())
	name = nonAlphaNum.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	ts := time.Now().Format("0405") // MMSS
	schema := fmt.Sprintf("t_%s_%s", name, ts)
	// Postgres identifiers are limited to 63 bytes.
	if len(schema) > 63 {
		schema = schema[:63]
	}
	return schema
}

func TestMigrator(t *testing.T) {
	t.Run("Migrate", func(t *testing.T) {
		t.Run("should fail on invalid feature name", func(t *testing.T) {
			// arrange
			var (
				conn     = postgres.NewTest(t)
				migrator = migrate.NewMigrator(conn)
				ddl      = fstest.MapFS{}
			)

			// act
			err := migrator.Migrate(t.Context(), "INVALID-NAME", ddl)

			// assert
			assert.Error(t, err)
		})

		t.Run("should fail on invalid migration filename", func(t *testing.T) {
			// arrange
			var (
				conn     = postgres.NewTest(t)
				schema   = testSchema(t)
				migrator = migrate.NewMigrator(conn)
				ddl      = fstest.MapFS{
					"bad_name.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
				}
			)

			// act
			err := migrator.Migrate(t.Context(), schema, ddl)

			// assert
			assert.Error(t, err)
		})

		t.Run("should fail on duplicate migration ids", func(t *testing.T) {
			// arrange
			var (
				conn     = postgres.NewTest(t)
				schema   = testSchema(t)
				migrator = migrate.NewMigrator(conn)
				ddl      = fstest.MapFS{
					"001_first.sql":  &fstest.MapFile{Data: []byte("SELECT 1;")},
					"001_second.sql": &fstest.MapFile{Data: []byte("SELECT 2;")},
				}
			)

			// act
			err := migrator.Migrate(t.Context(), schema, ddl)

			// assert
			assert.Error(t, err)
		})

		t.Run("should fail on erroneous migration", func(t *testing.T) {
			// arrange
			var (
				conn     = postgres.NewTest(t)
				schema   = testSchema(t)
				migrator = migrate.NewMigrator(conn)
				ddl      = fstest.MapFS{
					"001_bad_sql.sql": &fstest.MapFile{Data: []byte("THIS IS NOT VALID SQL;")},
				}
			)

			// act
			err := migrator.Migrate(t.Context(), schema, ddl)

			// assert
			assert.Error(t, err)
		})

		t.Run("should apply migrations in order", func(t *testing.T) {
			// arrange
			var (
				conn     = postgres.NewTest(t)
				ctx      = t.Context()
				schema   = testSchema(t)
				migrator = migrate.NewMigrator(conn)
				ddl      = fstest.MapFS{
					"002_add_column.sql":   &fstest.MapFile{Data: []byte("ALTER TABLE items ADD COLUMN description TEXT;")},
					"001_create_table.sql": &fstest.MapFile{Data: []byte("CREATE TABLE items (id SERIAL PRIMARY KEY, name TEXT NOT NULL);")},
				}
			)

			// act
			err := migrator.Migrate(ctx, schema, ddl)

			// assert
			assert.NoError(t, err)
			_, insertErr := conn.Exec(ctx,
				fmt.Sprintf(`INSERT INTO "%s".items (name, description) VALUES ('a', 'b')`, schema))
			assert.NoError(t, insertErr)
		})

		t.Run("should skip already applied migrations", func(t *testing.T) {
			// arrange
			var (
				conn     = postgres.NewTest(t)
				ctx      = t.Context()
				schema   = testSchema(t)
				migrator = migrate.NewMigrator(conn)
				ddl      = fstest.MapFS{
					"001_create_table.sql": &fstest.MapFile{Data: []byte("CREATE TABLE things (id SERIAL PRIMARY KEY);")},
				}
			)
			assert.NoError(t, migrator.Migrate(ctx, schema, ddl))

			// act — run the same migrations again
			err := migrator.Migrate(ctx, schema, ddl)

			// assert
			assert.NoError(t, err)
		})

		t.Run("should create isolated schemas per feature", func(t *testing.T) {
			// arrange
			var (
				conn     = postgres.NewTest(t)
				ctx      = t.Context()
				schemaA  = testSchema(t) + "_a"
				schemaB  = testSchema(t) + "_b"
				migrator = migrate.NewMigrator(conn)
				ddlA     = fstest.MapFS{
					"001_create.sql": &fstest.MapFile{Data: []byte("CREATE TABLE widgets (id SERIAL PRIMARY KEY);")},
				}
				ddlB = fstest.MapFS{
					"001_create.sql": &fstest.MapFile{Data: []byte("CREATE TABLE widgets (id SERIAL PRIMARY KEY);")},
				}
			)

			// act
			errA := migrator.Migrate(ctx, schemaA, ddlA)
			errB := migrator.Migrate(ctx, schemaB, ddlB)

			// assert — both succeed despite same table name
			assert.NoError(t, errA)
			assert.NoError(t, errB)
		})

		t.Run("should fail on checksum mismatch", func(t *testing.T) {
			// arrange
			var (
				conn     = postgres.NewTest(t)
				ctx      = t.Context()
				schema   = testSchema(t)
				migrator = migrate.NewMigrator(conn)
				original = fstest.MapFS{
					"001_create.sql": &fstest.MapFile{Data: []byte("CREATE TABLE checksumtest (id SERIAL PRIMARY KEY);")},
				}
				modified = fstest.MapFS{
					"001_create.sql": &fstest.MapFile{Data: []byte("CREATE TABLE checksumtest (id SERIAL PRIMARY KEY, name TEXT);")},
				}
			)
			assert.NoError(t, migrator.Migrate(ctx, schema, original))

			// act — re-run with modified content for same id
			err := migrator.Migrate(ctx, schema, modified)

			// assert
			assert.Error(t, err)
		})

		t.Run("should succeed with no migration files", func(t *testing.T) {
			// arrange
			var (
				conn     = postgres.NewTest(t)
				schema   = testSchema(t)
				migrator = migrate.NewMigrator(conn)
				ddl      = fstest.MapFS{}
			)

			// act
			err := migrator.Migrate(t.Context(), schema, ddl)

			// assert
			assert.NoError(t, err)
		})
	})
}

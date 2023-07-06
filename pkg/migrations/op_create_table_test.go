package migrations_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"pg-roll/pkg/migrations"
	"pg-roll/pkg/roll"
	"pg-roll/pkg/state"

	"github.com/google/go-cmp/cmp"
	"github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"golang.org/x/exp/slices"
)

const (
	postgresImage = "postgres:15.3"
	viewName      = "users"
	schema        = "public"
)

func TestViewForNewVersionIsCreatedAfterMigrationStart(t *testing.T) {
	t.Parallel()

	withMigratorAndConnectionToContainer(t, func(mig *roll.Roll, db *sql.DB) {
		ctx := context.Background()
		version := "1_create_table"
		versionSchema := roll.VersionedSchemaName(schema, version)

		if err := mig.Start(ctx, &migrations.Migration{Name: version, Operations: migrations.Operations{createTableOp()}}); err != nil {
			t.Fatalf("Failed to start migration: %v", err)
		}

		var exists bool
		err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM pg_catalog.pg_views
			WHERE schemaname = $1
			AND viewname = $2
		) `, versionSchema, viewName).Scan(&exists)
		if err != nil {
			t.Fatal(err)
		}

		if !exists {
			t.Fatalf("Expected view to exist")
		}
	})
}

func TestRecordsCanBeInsertedIntoAndReadFromNewViewAfterMigrationStart(t *testing.T) {
	t.Parallel()

	withMigratorAndConnectionToContainer(t, func(mig *roll.Roll, db *sql.DB) {
		ctx := context.Background()
		version := "1_create_table"
		versionSchema := roll.VersionedSchemaName(schema, version)

		if err := mig.Start(ctx, &migrations.Migration{Name: version, Operations: migrations.Operations{createTableOp()}}); err != nil {
			t.Fatalf("Failed to start migration: %v", err)
		}

		//
		// Insert records via the view
		//
		sql := fmt.Sprintf(`INSERT INTO %s.%s (id, name) VALUES ($1, $2)`,
			pq.QuoteIdentifier(versionSchema),
			pq.QuoteIdentifier(viewName))

		insertStmt, err := db.Prepare(sql)
		if err != nil {
			t.Fatal(err)
		}
		defer insertStmt.Close()

		type user struct {
			ID   int
			Name string
		}
		inserted := []user{{ID: 1, Name: "Alice"}, {ID: 2, Name: "Bob"}}

		for _, v := range inserted {
			_, err = insertStmt.Exec(v.ID, v.Name)
			if err != nil {
				t.Fatal(err)
			}
		}

		//
		// Read the records back via the view
		//
		sql = fmt.Sprintf(`SELECT id, name FROM %q.%q`, versionSchema, viewName)
		rows, err := db.Query(sql)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()

		var retrieved []user
		for rows.Next() {
			var user user
			if err := rows.Scan(&user.ID, &user.Name); err != nil {
				t.Fatal(err)
			}
			retrieved = append(retrieved, user)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}

		if !slices.Equal(inserted, retrieved) {
			t.Error(cmp.Diff(inserted, retrieved))
		}
	})
}

func TestViewSchemaAndTableAreDroppedAfterMigrationRevert(t *testing.T) {
	t.Parallel()

	withMigratorAndConnectionToContainer(t, func(mig *roll.Roll, db *sql.DB) {
		ctx := context.Background()
		version := "1_create_table"
		versionSchema := roll.VersionedSchemaName(schema, version)

		migration := &migrations.Migration{Name: version, Operations: migrations.Operations{createTableOp()}}

		if err := mig.Start(ctx, migration); err != nil {
			t.Fatalf("Failed to start migration: %v", err)
		}

		if err := mig.Rollback(ctx); err != nil {
			t.Fatalf("Failed to revert migration: %v", err)
		}

		var exists bool
		//
		// Check that the new table has been dropped
		//
		tableName := migrations.TemporaryName(viewName)
		err := db.QueryRow(`
    SELECT EXISTS(
      SELECT 1
      FROM pg_catalog.pg_tables
      WHERE tablename = $1
    )`, tableName).Scan(&exists)
		if err != nil {
			t.Fatal(err)
		}

		if exists {
			t.Errorf("Expected table %q to not exist", tableName)
		}

		//
		// Check that the view in the new schema has been dropped
		//
		err = db.QueryRow(`
    SELECT EXISTS (
      SELECT 1
      FROM pg_catalog.pg_views
      WHERE schemaname = $1
      AND viewname = $2
    ) `, versionSchema, viewName).Scan(&exists)
		if err != nil {
			t.Fatal(err)
		}

		if exists {
			t.Errorf("Expected view %q to not exist", viewName)
		}

		//
		// Check that the new schema has been dropped
		//
		err = db.QueryRow(`
    SELECT EXISTS(
      SELECT 1
      FROM pg_catalog.pg_namespace
      WHERE nspname = $1
    )`, versionSchema).Scan(&exists)
		if err != nil {
			t.Fatal(err)
		}

		if exists {
			t.Errorf("Expected schema %q to not exist", version)
		}
	})
}

func createTableOp() *migrations.OpCreateTable {
	return &migrations.OpCreateTable{
		Name: viewName,
		Columns: []migrations.Column{
			{
				Name:       "id",
				Type:       "integer",
				PrimaryKey: true,
			},
			{
				Name:   "name",
				Type:   "varchar(255)",
				Unique: true,
			},
		},
	}
}

func withMigratorAndConnectionToContainer(t *testing.T, fn func(mig *roll.Roll, db *sql.DB)) {
	t.Helper()
	ctx := context.Background()

	waitForLogs := wait.
		ForLog("database system is ready to accept connections").
		WithOccurrence(2).
		WithStartupTimeout(5 * time.Second)

	ctr, err := postgres.RunContainer(ctx,
		testcontainers.WithImage(postgresImage),
		testcontainers.WithWaitStrategy(waitForLogs),
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := ctr.Terminate(ctx); err != nil {
			t.Fatalf("Failed to terminate container: %v", err)
		}
	})

	cStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}

	st, err := state.New(ctx, cStr, "pgroll")
	if err != nil {
		t.Fatal(err)
	}
	err = st.Init(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mig, err := roll.New(ctx, cStr, "public", st)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := mig.Close(); err != nil {
			t.Fatalf("Failed to close migrator connection: %v", err)
		}
	})

	db, err := sql.Open("postgres", cStr)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("Failed to close database connection: %v", err)
		}
	})

	fn(mig, db)
}
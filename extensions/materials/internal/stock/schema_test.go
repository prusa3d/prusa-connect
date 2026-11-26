package stock

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"

	"github.com/prusa3d/materials/internal/dbtest"

	"github.com/prusa3d/prusa-connect/sdk/moduledb"
)

func TestTheSchemaAppliesOnEachEngine(t *testing.T) {
	for _, engine := range dbtest.Engines(t) {
		t.Run(string(engine.Name), func(t *testing.T) {
			db := dbtest.Open(t, engine.Grant(t))
			s := New(db, engine.Name)
			now := s.at(time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC))

			for _, q := range []struct {
				sql  string
				args []any
			}{
				{`INSERT INTO brands (uuid, slug, name) VALUES ($1, $2, $3)`, []any{"b1", "prusament", "Prusament"}},
				{`INSERT INTO containers (uuid, slug, brand_slug, name, empty_weight_g) VALUES ($1, $2, $3, $4, $5)`, []any{"c1", "spool", "prusament", "Spool", 230.5}},
				{`INSERT INTO materials (uuid, slug, brand_slug, name, class, type, abbreviation, color_rgba, density, min_print_temp, drying_time_min)
				  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`, []any{"m1", "pla", "prusament", "PLA", "FFF", "PLA", "PLA", "ff0000ff", 1.24, 215, 240}},
				{`INSERT INTO packages (uuid, slug, class, material_slug, container_slug, net_weight_g, diameter_um, length_mm, gtin)
				  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, []any{"p1", "pla-1kg", "FFF", "pla", "spool", 1000.0, 1750, 330000, "0123"}},
				{`INSERT INTO catalogue_meta (id, data_hash, last_modified, brands, materials, packages, containers, skipped, imported_at)
				  VALUES (1, $1, $2, $3, $4, $5, $6, $7, $8)`, []any{"h", "lm", 1, 1, 1, 1, 0, now}},
				{`INSERT INTO custom_materials (uuid, owner, name, created_at, updated_at) VALUES ($1, $2, $3, $4, $4)`, []any{"u1", "alice", "Mine", now}},
				{`INSERT INTO favourites (principal, material_uuid, starred_at) VALUES ($1, $2, $3)`, []any{"alice", "m1", now}},
				{`INSERT INTO spools (material_uuid, material_source, remaining_weight_g, created_at, updated_at) VALUES ($1, $2, $3, $4, $4)`, []any{"m1", "catalogue", 750.5, now}},
			} {
				if _, err := db.Exec(q.sql, q.args...); err != nil {
					t.Fatalf("%s: %v", q.sql, err)
				}
			}

			var id int64
			var remaining float64
			var deleted any
			if err := db.QueryRow(`SELECT id, remaining_weight_g, deleted_at FROM spools`).
				Scan(&id, &remaining, &deleted); err != nil {
				t.Fatal(err)
			}
			if id != 1 || remaining != 750.5 || deleted != nil {
				t.Fatalf("spool = %d, %v, %v", id, remaining, deleted)
			}

			var slug string
			var density float64
			if err := db.QueryRow(`SELECT slug, density FROM materials WHERE uuid = $1`, "m1").
				Scan(&slug, &density); err != nil || slug != "pla" ||
				density != 1.24 {
				t.Fatalf("material = %q, %v, %v", slug, density, err)
			}

			var n int
			for _, table := range []string{"brands", "containers", "packages", "catalogue_meta", "custom_materials", "favourites"} {
				if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil || n != 1 {
					t.Fatalf("%s rows = %d, %v", table, n, err)
				}
			}
		})
	}
}

func TestOnlyAUniqueIndexRefusalIsUnique(t *testing.T) {
	for _, engine := range dbtest.Engines(t) {
		t.Run(string(engine.Name), func(t *testing.T) {
			s, ctx := newStore(t, engine), context.Background()

			if _, err := s.CreateLocation(ctx, "Box"); err != nil {
				t.Fatal(err)
			}

			_, err := s.db.ExecContext(ctx, `INSERT INTO locations (name, created_at, updated_at) VALUES ($1, $2, $2)`,
				"box", s.at(time.Now()))
			if !unique(err) {
				t.Fatalf("a second box: %v", err)
			}

			if _, err = s.db.ExecContext(ctx, `INSERT INTO no_such_table VALUES (1)`); err == nil || unique(err) {
				t.Fatalf("a missing table: %v", err)
			}
		})
	}
}

func TestMigrationGivesExistingSpoolsAUUID(t *testing.T) {
	for _, engine := range dbtest.Engines(t) {
		t.Run(string(engine.Name), func(t *testing.T) {
			grant := engine.Grant(t)
			all := dbtest.Schema().Migrations

			// The database as the first release left it, with three spools in it.
			first := fstest.MapFS{}

			for _, dir := range []string{"migrations/sqlite", "migrations/postgres"} {
				body, err := fs.ReadFile(all, dir+"/20261028T091554_materials.sql")
				if err != nil {
					t.Fatal(err)
				}

				first[dir+"/20261028T091554_materials.sql"] = &fstest.MapFile{Data: body}
			}

			old, _, err := moduledb.Open(context.Background(), grant, moduledb.Schema{Migrations: first})
			if err != nil {
				t.Fatal(err)
			}

			now := New(old, engine.Name).at(time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC))

			for range 3 {
				if _, err = old.Exec(
					`INSERT INTO spools (material_uuid, material_source, remaining_weight_g, created_at, updated_at)
					VALUES ('m1', 'catalogue', 1, $1, $1)`,
					now,
				); err != nil {
					t.Fatal(err)
				}
			}

			if err = old.Close(); err != nil {
				t.Fatal(err)
			}

			db := dbtest.Open(t, grant)

			rows, err := db.Query(`SELECT uuid, source FROM spools`)
			if err != nil {
				t.Fatal(err)
			}

			defer rows.Close()

			seen := map[string]bool{}

			for rows.Next() {
				var id, source string
				if err = rows.Scan(&id, &source); err != nil {
					t.Fatal(err)
				}

				parsed, perr := uuid.Parse(id)
				if perr != nil || parsed.Version() != 4 || parsed.Variant() != uuid.RFC4122 ||
					id != strings.ToLower(id) ||
					source != "manual" {
					t.Fatalf("spool = %q, %q, %v", id, source, perr)
				}

				seen[id] = true
			}

			if err = rows.Err(); err != nil {
				t.Fatal(err)
			}

			if len(seen) != 3 {
				t.Fatalf("the spools share uuids: %v", seen)
			}

			if _, err = db.Exec(
				`INSERT INTO spools (material_uuid, material_source, remaining_weight_g, created_at, updated_at, uuid)
				SELECT 'm1', 'catalogue', 1, created_at, updated_at, uuid FROM spools LIMIT 1`,
			); err == nil {
				t.Fatal("two spools may share a uuid")
			}
		})
	}
}

func TestMigrationRepairsDiametersReadAsWholeMillimetres(t *testing.T) {
	for _, engine := range dbtest.Engines(t) {
		t.Run(string(engine.Name), func(t *testing.T) {
			grant := engine.Grant(t)
			all := dbtest.Schema().Migrations

			// The database as 0.2.8 left it, after a catalogue that writes
			// millimetres was read as micrometres: 1.75 became 1.
			before := fstest.MapFS{}

			for _, dir := range []string{"migrations/sqlite", "migrations/postgres"} {
				for _, name := range []string{"20261028T091554_materials.sql", "20261102T080000_printer_tools.sql"} {
					body, err := fs.ReadFile(all, dir+"/"+name)
					if err != nil {
						t.Fatal(err)
					}

					before[dir+"/"+name] = &fstest.MapFile{Data: body}
				}
			}

			old, _, err := moduledb.Open(context.Background(), grant, moduledb.Schema{Migrations: before})
			if err != nil {
				t.Fatal(err)
			}

			now := New(old, engine.Name).at(time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC))

			for _, q := range []struct {
				sql  string
				args []any
			}{
				{`INSERT INTO packages (uuid, slug, material_slug, diameter_um) VALUES
				  ('p1', 'a', 'm', 1), ('p2', 'b', 'm', 2), ('p3', 'c', 'm', 3), ('p4', 'd', 'm', 1750),
				  ('p5', 'e', 'm', NULL)`, nil},
				{`INSERT INTO spools (uuid, material_uuid, material_source, remaining_weight_g, diameter_um, created_at, updated_at)
				  VALUES ('s1', 'm1', 'catalogue', 1, 1, $1, $1), ('s2', 'm1', 'catalogue', 1, 1750, $1, $1),
				  ('s3', 'm1', 'catalogue', 1, NULL, $1, $1)`, []any{now}},
			} {
				if _, err = old.Exec(q.sql, q.args...); err != nil {
					t.Fatalf("%s: %v", q.sql, err)
				}
			}

			if err = old.Close(); err != nil {
				t.Fatal(err)
			}

			db := dbtest.Open(t, grant)

			for q, want := range map[string]string{
				`SELECT uuid, diameter_um FROM packages ORDER BY uuid`: "p1:1750 p2:2850 p3:3000 p4:1750 p5:<nil> ",
				`SELECT uuid, diameter_um FROM spools ORDER BY uuid`:   "s1:1750 s2:1750 s3:<nil> ",
			} {
				if got := diameters(t, db, q); got != want {
					t.Errorf("%s = %q, want %q", q, got, want)
				}
			}
		})
	}
}

// diameters lists each row of q as key:diameter.
func diameters(t *testing.T, db *sql.DB, q string) string {
	t.Helper()

	rows, err := db.Query(q)
	if err != nil {
		t.Fatal(err)
	}

	defer rows.Close()

	var got strings.Builder

	for rows.Next() {
		var (
			key string
			d   *int
		)
		if err = rows.Scan(&key, &d); err != nil {
			t.Fatal(err)
		}

		fmt.Fprintf(&got, "%s:%v ", key, deref(d))
	}

	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}

	return got.String()
}

func deref(n *int) any {
	if n == nil {
		return "<nil>"
	}

	return *n
}

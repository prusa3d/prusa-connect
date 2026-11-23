package resource

import (
	"fmt"
	"slices"
	"time"

	"github.com/prusa3d/prusa-connect/internal/app/config/install"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"
)

// A module's own database is kept when its grant ends. A removal, and an
// update that moves the database to another place, leave the old database
// where it is and record it in the machine-owned overlay, so the operator
// sees it and can purge it. The record holds the connection string, which is
// a credential. The overlay already holds credentials and has mode 0600.

// Why a database was left behind.
const (
	// LeftRemoved is a database whose module was removed.
	LeftRemoved = "removed"
	// LeftMoved is a database an update moved away from: after the update the
	// module keeps its data in another place.
	LeftMoved = "moved"
	// LeftNotGranted is a database an update did not grant again: an update
	// installed without approve, or a release that asks for no database.
	LeftNotGranted = "not_granted"
)

// LeftDatabase is a module's own database that no declaration grants.
type LeftDatabase struct {
	// Instance is the instance that was granted it, as kind/name.
	Instance string `json:"instance"`
	// Type is the module's type, namespace and name. A later install must
	// have the same type before it gets the database back.
	Type string `json:"type"`
	// Database is the grant as the declaration held it: the engine, the
	// connection string for Postgres, and whether this server made it.
	Database install.DatabaseEntry `json:"database"`
	// LeftAt is when the grant ended.
	LeftAt time.Time `json:"left_at"`
	// Why is LeftRemoved, LeftMoved or LeftNotGranted.
	Why string `json:"why"`
}

// Grant is the database as a declaration grants it.
func (l LeftDatabase) Grant() *install.Database {
	return &install.Database{
		Engine:      wirerpc.DatabaseEngine(l.Database.Engine),
		DSN:         l.Database.DSN,
		Provisioned: l.Database.Provisioned,
	}
}

// Ref is the instance that was granted the database.
func (l LeftDatabase) Ref() (ref.Ref, error) {
	return ref.Parse(l.Instance)
}

// LeftBehind records a grant as left behind by an instance.
func LeftBehind(instance ref.Ref, typeName string, grant *install.Database, why string, at time.Time) LeftDatabase {
	return LeftDatabase{
		Instance: instance.String(),
		Type:     typeName,
		Database: *grant.Entry(),
		LeftAt:   at.UTC(),
		Why:      why,
	}
}

// SamePlace reports whether two grants name the same database: one
// instance's SQLite file, or one database on one Postgres server.
func SamePlace(instance ref.Ref, left LeftDatabase, grant *install.Database) bool {
	if grant == nil || left.Instance != instance.String() {
		return false
	}

	held := left.Grant()
	if held.Engine != grant.Engine {
		return false
	}

	// A SQLite file belongs to the instance: database.sqlite in its data
	// directory, which is named after the instance.
	if held.Engine == wirerpc.DatabaseSQLite {
		return true
	}

	return held.Server() == grant.Server()
}

// LeftDatabases is every database the overlay records as left behind, oldest
// first.
func LeftDatabases(overlayPath string) ([]LeftDatabase, error) {
	overlay, err := readOverlayFile(overlayPath)
	if err != nil {
		return nil, err
	}

	return slices.Clone(overlay.Databases), nil
}

// ForgetLeftDatabase removes one record from the overlay, after a purge, and
// reports whether the overlay held it.
func ForgetLeftDatabase(overlayPath string, forgotten LeftDatabase) (bool, error) {
	overlayWriting.Lock()
	defer overlayWriting.Unlock()

	overlay, err := readOverlayFile(overlayPath)
	if err != nil {
		return false, err
	}

	kept := slices.DeleteFunc(slices.Clone(overlay.Databases), func(held LeftDatabase) bool {
		return held.Instance == forgotten.Instance && held.Database == forgotten.Database
	})

	if len(kept) == len(overlay.Databases) {
		return false, nil
	}

	overlay.Databases = kept

	if err = writeOverlayFile(overlayPath, overlay); err != nil {
		return false, fmt.Errorf("forget the database: %w", err)
	}

	return true, nil
}

// LeftChange is the change one install or removal makes to the records: the
// databases it grants again, and the one it leaves behind.
type LeftChange struct {
	// Take is the record of each database the install grants again.
	Take []LeftDatabase
	// Leave is the database whose grant ends with the change, and nil for
	// none.
	Leave *LeftDatabase
}

// applied is the records after the change.
func (c LeftChange) applied(held []LeftDatabase) []LeftDatabase {
	out := slices.DeleteFunc(slices.Clone(held), func(one LeftDatabase) bool {
		return slices.ContainsFunc(c.Take, func(taken LeftDatabase) bool {
			return taken.Instance == one.Instance && taken.Database == one.Database
		})
	})

	if c.Leave != nil {
		out = append(out, *c.Leave)
	}

	return out
}

package stock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/prusa3d/materials/internal/consume"
)

// Reading is what one printer tool reports of the filament it fed: its
// odometer, and the grams used that the tag of its spool holds. A field it
// does not report is nil, or empty.
type Reading struct {
	E            *float64
	ID           *int64
	TagConsumedG *float64
	// TagUUID is the uuid of the tag the tool reads.
	TagUUID string
}

// Consume takes what a tool fed since its last reading off the spool on it.
// The reading is kept with the tool's assignment, so a tool with none keeps
// nothing and charges nothing.
//
// The tag's grams are the spool's total while the tool reads that spool's own
// tag, and the odometer counts otherwise. The spool loses only what its total
// grew by, never below 0.
func (s *Store) Consume(ctx context.Context, deviceID, toolID string, r Reading) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	var (
		spoolID *int64
		prevE   *float64
		prevID  *int64
	)

	err = tx.QueryRowContext(ctx, `SELECT spool_id, odometer_e, odometer_id FROM assignments
		WHERE device_id = $1 AND tool_id = $2`, deviceID, toolID).Scan(&spoolID, &prevE, &prevID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("read the tool's odometer: %w", err)
	}

	mm, counted, err := keepReading(ctx, tx, deviceID, toolID, prevE, prevID, r)
	if err != nil || !counted {
		return err
	}

	took := false

	if spoolID != nil {
		if took, err = takeOff(ctx, tx, *spoolID, mm, r); err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	if took {
		s.spoolChanged(true, *spoolID)
	}

	return nil
}

// keepReading moves the tool's odometer to r, and answers the millimetres it
// fed since the reading kept. counted is false when the reading moves nothing:
// an older one, the one kept with no tag grams, or one another writer counted
// first. The guard on the last reading lets only one of two writers holding
// it count what came after.
func keepReading(ctx context.Context, tx *sql.Tx, deviceID, toolID string, prevE *float64, prevID *int64, r Reading,
) (mm float64, counted bool, err error) {
	if r.E == nil {
		return 0, true, nil
	}

	var stale bool
	if mm, stale = consume.Delta(prevE, prevID, *r.E, r.ID); stale {
		return 0, false, nil
	}

	if sameReading(prevE, prevID, *r.E, r.ID) {
		// The reading kept, sent again: only tag grams can move the spool.
		return 0, r.TagConsumedG != nil, nil
	}

	res, err := tx.ExecContext(ctx, `UPDATE assignments SET odometer_e = $1, odometer_id = $2
		WHERE device_id = $3 AND tool_id = $4
			AND odometer_e IS NOT DISTINCT FROM $5 AND odometer_id IS NOT DISTINCT FROM $6`,
		*r.E, r.ID, deviceID, toolID, prevE, prevID)
	if err != nil {
		return 0, false, fmt.Errorf("keep the tool's odometer: %w", err)
	}

	kept, err := res.RowsAffected()
	if err != nil {
		return 0, false, fmt.Errorf("keep the tool's odometer: %w", err)
	}

	return mm, kept > 0, nil
}

// sameReading is true when a reading is the one the tool holds already.
func sameReading(prevE *float64, prevID *int64, e float64, id *int64) bool {
	return prevE != nil && *prevE == e && (prevID == nil) == (id == nil) && (id == nil || *prevID == *id)
}

// takeOff moves a spool's consumed total forward, and its remaining weight
// down by as much: the tag's grams while the tool reads the spool's own tag,
// else what the odometer fed. took is false when nothing moved.
func takeOff(ctx context.Context, tx *sql.Tx, id int64, mm float64, r Reading) (took bool, err error) {
	var (
		uuid, typ, source string
		density           *float64
		diameterUM        *int
		consumed          float64
	)

	if err = tx.QueryRowContext(ctx, `SELECT uuid, source, type, density, diameter_um, consumed_weight_g
		FROM spools WHERE id = $1`, id).
		Scan(&uuid, &source, &typ, &density, &diameterUM, &consumed); err != nil {
		return false, fmt.Errorf("read the spool on the tool: %w", err)
	}

	total := consumed + consume.Grams(mm, density, typ, diameterUM)
	// Only a spool read off a tag has a uuid a tag can name: the module mints
	// a manual spool's, and anyone who reads the stock sees it.
	if r.TagConsumedG != nil && r.TagUUID == uuid && source == SourcePrinter {
		total = *r.TagConsumedG
	}

	if total <= consumed {
		return false, nil
	}

	// The remaining weight goes down from the row as it stands, so a weight a
	// person types between the read above and this write is kept.
	_, err = tx.ExecContext(ctx, `UPDATE spools SET consumed_weight_g = $1,
		remaining_weight_g = CASE WHEN remaining_weight_g > $2 THEN remaining_weight_g - $2 ELSE 0 END WHERE id = $3`,
		total, total-consumed, id)
	if err != nil {
		return false, fmt.Errorf("take the filament off the spool: %w", err)
	}

	return true, nil
}

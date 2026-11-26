package stock

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/prusa3d/materials/internal/colour"
)

// Where a spool came from.
const (
	SourceManual  = "manual"
	SourcePrinter = "printer"
)

// SpoolDraft is what the person types when adding spools.
type SpoolDraft struct {
	MaterialUUID     string   `json:"material_uuid"`
	PackageUUID      *string  `json:"package_uuid,omitempty"`
	FullWeightG      *float64 `json:"full_weight_g,omitempty"`
	EmptyWeightG     *float64 `json:"empty_weight_g,omitempty"`
	LengthMM         *int     `json:"length_mm,omitempty"`
	RemainingWeightG float64  `json:"remaining_weight_g"`
	// Condition is kept in the store but not offered by the API; empty is "new".
	Condition  string `json:"-"`
	LocationID *int64 `json:"location_id,omitempty"`
	Notes      string `json:"notes"`
}

// Spool is one spool in stock, with the display fields copied from its material.
type Spool struct {
	ID int64 `json:"id"`
	// UUID is the spool's own identity. A spool a printer reports carries the uuid of its tag.
	UUID             string    `json:"uuid"`
	Source           string    `json:"source"`
	MaterialUUID     string    `json:"material_uuid"`
	MaterialSource   string    `json:"material_source"`
	MaterialName     string    `json:"material_name"`
	BrandName        string    `json:"brand_name"`
	Class            string    `json:"class"`
	Type             string    `json:"type"`
	ColorRGBA        string    `json:"color_rgba"`
	Density          *float64  `json:"density,omitempty"`
	DiameterUM       *int      `json:"diameter_um,omitempty"`
	PackageUUID      *string   `json:"package_uuid,omitempty"`
	FullWeightG      *float64  `json:"full_weight_g,omitempty"`
	EmptyWeightG     *float64  `json:"empty_weight_g,omitempty"`
	LengthMM         *int      `json:"length_mm,omitempty"`
	RemainingWeightG float64   `json:"remaining_weight_g"`
	Condition        string    `json:"-"`
	LocationID       *int64    `json:"location_id,omitempty"`
	Location         string    `json:"location"`
	Notes            string    `json:"notes"`
	CreatedBy        string    `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	// ArchivedAt is when the spool was archived, nil for one in stock.
	ArchivedAt        *time.Time `json:"archived_at"`
	ArchiveReason     string     `json:"archive_reason,omitempty"`
	ArchiveReasonNote string     `json:"archive_reason_note,omitempty"`
	// Tool is the printer tool the spool is on, nil for none.
	Tool *ToolRef `json:"tool"`
}

// ToolRef names one printer tool.
type ToolRef struct {
	DeviceID string `json:"device_id"`
	ToolID   string `json:"tool_id"`
	// Gone is true when the printer no longer has the tool.
	Gone bool `json:"gone"`
}

// SpoolPatch changes the fields that are set.
type SpoolPatch struct {
	RemainingWeightG *float64 `json:"remaining_weight_g,omitempty"`
	Condition        *string  `json:"-"`
	// LocationID moves the spool to a location; 0 takes it out of any.
	LocationID *int64  `json:"location_id,omitempty"`
	Notes      *string `json:"notes,omitempty"`
	// Archived true archives the spool, false brings it back to stock and
	// clears the reason and the note.
	Archived *bool `json:"archived,omitempty"`
	// ArchiveReason and ArchiveReasonNote are set on an archived spool only;
	// "" clears them.
	ArchiveReason     *string `json:"archive_reason,omitempty"`
	ArchiveReasonNote *string `json:"archive_reason_note,omitempty"`
}

const spoolColumns = `s.id, s.uuid, s.source, s.material_uuid, s.material_source, s.material_name, s.brand_name, s.class, s.type,
	s.color_rgba, s.density, s.diameter_um, s.package_uuid, s.full_weight_g, s.empty_weight_g, s.length_mm,
	s.remaining_weight_g, s.condition, s.location_id, COALESCE(l.name, ''), s.notes, s.created_by, s.created_at,
	s.updated_at, s.archived_at, COALESCE(s.archive_reason, ''), COALESCE(s.archive_reason_note, ''),
	a.device_id, a.tool_id`

// spoolsFrom is the spools with the name of the location each is kept at and
// the tool each is on.
const spoolsFrom = `spools s LEFT JOIN locations l ON l.id = s.location_id
	LEFT JOIN assignments a ON a.spool_id = s.id`

func scanSpool(row interface{ Scan(...any) error }) (Spool, error) {
	var (
		sp               Spool
		created, updated any
		archived         any
		device, tool     sql.NullString
	)

	err := row.Scan(
		&sp.ID,
		&sp.UUID,
		&sp.Source,
		&sp.MaterialUUID,
		&sp.MaterialSource,
		&sp.MaterialName,
		&sp.BrandName,
		&sp.Class,
		&sp.Type,
		&sp.ColorRGBA,
		&sp.Density,
		&sp.DiameterUM,
		&sp.PackageUUID,
		&sp.FullWeightG,
		&sp.EmptyWeightG,
		&sp.LengthMM,
		&sp.RemainingWeightG,
		&sp.Condition,
		&sp.LocationID,
		&sp.Location,
		&sp.Notes,
		&sp.CreatedBy,
		&created,
		&updated,
		&archived,
		&sp.ArchiveReason,
		&sp.ArchiveReasonNote,
		&device,
		&tool,
	)
	if err != nil {
		return sp, err
	}

	if device.Valid {
		sp.Tool = &ToolRef{DeviceID: device.String, ToolID: tool.String}
	}

	if sp.CreatedAt, err = parseTime(created); err != nil {
		return sp, err
	}

	if sp.UpdatedAt, err = parseTime(updated); err != nil || archived == nil {
		return sp, err
	}

	at, err := parseTime(archived)
	sp.ArchivedAt = &at

	return sp, err
}

func validCondition(c string) bool { return c == "new" || c == "partially_used" }

// MaxWeightG is the heaviest weight a spool records, 100 kg. Far heavier
// ones overflow the stock total the list sums.
const MaxWeightG = 100_000

// badWeight is true for a weight under 0, over MaxWeightG or not a number.
func badWeight(w float64) bool { return !(w >= 0 && w <= MaxWeightG) }

// invalidWeights is true for a bad weight or a remaining weight over a known net weight.
func invalidWeights(remaining float64, full, empty *float64) bool {
	if badWeight(remaining) || (full != nil && badWeight(*full)) || (empty != nil && badWeight(*empty)) {
		return true
	}

	return full != nil && empty != nil && remaining > *full-*empty
}

// MaxQuantity is the most spools one add may make.
const MaxQuantity = 100

// AddSpools copies the material into quantity new spools in one transaction.
// ErrFull when that would pass limit live spools.
func (s *Store) AddSpools(ctx context.Context, principal string, d SpoolDraft, quantity, limit int) ([]Spool, error) {
	if d.Condition == "" {
		d.Condition = "new"
	}

	if quantity < 1 || quantity > MaxQuantity || !validCondition(d.Condition) ||
		invalidWeights(d.RemainingWeightG, d.FullWeightG, d.EmptyWeightG) || !fits(d.Notes, maxNotes) {
		return nil, ErrInvalid
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	base := Spool{
		Source:           SourceManual,
		MaterialUUID:     d.MaterialUUID,
		MaterialSource:   "catalogue",
		PackageUUID:      d.PackageUUID,
		FullWeightG:      d.FullWeightG,
		EmptyWeightG:     d.EmptyWeightG,
		LengthMM:         d.LengthMM,
		RemainingWeightG: d.RemainingWeightG,
		Condition:        d.Condition,
		Notes:            d.Notes,
		CreatedBy:        principal,
	}

	if err = copyMaterial(ctx, tx, principal, d, &base); err != nil {
		return nil, err
	}

	if d.LocationID != nil {
		if base.Location, err = keptAt(ctx, tx, *d.LocationID); err != nil {
			return nil, err
		}

		base.LocationID = d.LocationID
	}

	if err = countLive(ctx, tx, quantity, limit); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	base.CreatedAt, base.UpdatedAt = now, now
	out := make([]Spool, 0, quantity)

	for range quantity {
		sp := base
		sp.UUID = uuid.NewString()

		if err = s.insertSpool(ctx, tx, &sp); err != nil {
			return nil, err
		}

		out = append(out, sp)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	for _, sp := range out {
		s.spoolChanged(false, sp.ID)
	}

	return out, nil
}

// insertSpool writes one new spool and fills its id.
func (s *Store) insertSpool(ctx context.Context, tx *sql.Tx, sp *Spool) error {
	err := tx.QueryRowContext(ctx, `INSERT INTO spools (material_uuid, material_source, material_name, brand_name, class,
		type, color_rgba, density, diameter_um, package_uuid, full_weight_g, empty_weight_g, length_mm, remaining_weight_g,
		condition, location_id, notes, created_by, created_at, updated_at, color_family, uuid, source)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$19,$20,$21,$22) RETURNING id`,
		sp.MaterialUUID, sp.MaterialSource, sp.MaterialName, sp.BrandName, sp.Class, sp.Type, sp.ColorRGBA, sp.Density,
		sp.DiameterUM, sp.PackageUUID, sp.FullWeightG, sp.EmptyWeightG, sp.LengthMM, sp.RemainingWeightG,
		sp.Condition, sp.LocationID, sp.Notes, sp.CreatedBy, s.at(sp.CreatedAt), colour.Family(sp.ColorRGBA), sp.UUID,
		sp.Source).
		Scan(&sp.ID)
	if err != nil {
		return fmt.Errorf("insert spool: %w", err)
	}

	return nil
}

// countLive refuses with ErrFull when adding more spools would pass limit live
// spools. An archived spool does not count.
//
// ponytail: count-then-insert is not locked, two concurrent adds on Postgres may pass the limit by one batch.
func countLive(ctx context.Context, tx *sql.Tx, more, limit int) error {
	var live int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM spools WHERE deleted_at IS NULL AND archived_at IS NULL`).
		Scan(&live); err != nil {
		return fmt.Errorf("count spools: %w", err)
	}

	if live+more > limit {
		return ErrFull
	}

	return nil
}

// Spool answers one spool, deleted or not, and live true while it is in
// stock. ErrNotFound for none.
func (s *Store) Spool(ctx context.Context, id int64) (sp Spool, live bool, err error) {
	sp, err = s.spool(ctx, s.db, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Spool{}, false, ErrNotFound
	}

	if err != nil {
		return Spool{}, false, err
	}

	err = s.db.QueryRowContext(ctx, `SELECT deleted_at IS NULL FROM spools WHERE id = $1`, id).Scan(&live)
	if err != nil {
		return Spool{}, false, fmt.Errorf("read spool: %w", err)
	}

	return sp, live, nil
}

// spool reads one spool.
func (s *Store) spool(ctx context.Context, q interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}, id int64,
) (Spool, error) {
	sp, err := scanSpool(q.QueryRowContext(ctx, `SELECT `+spoolColumns+` FROM `+spoolsFrom+` WHERE s.id = $1`, id))
	if err != nil {
		return Spool{}, fmt.Errorf("read spool: %w", err)
	}

	return sp, nil
}

// ErrMergeRefused refuses a merge that is not of a manual spool into a
// printer spool of the same material.
var ErrMergeRefused = errors.New("only a manual spool merges, into a printer spool of the same material")

// MergeSpool merges the manual spool from into the printer spool into, as
// one physical spool. into keeps its uuid, its source and its values. It
// takes the location and the notes of from where it has none, and from
// leaves stock. ErrSpoolAssigned when from is on a tool, ErrSpoolArchived
// when either is archived.
func (s *Store) MergeSpool(ctx context.Context, from, into int64, now time.Time) (Spool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Spool{}, fmt.Errorf("begin: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	var pair [2]Spool

	for i, id := range []int64{from, into} {
		pair[i], err = scanSpool(tx.QueryRowContext(ctx,
			`SELECT `+spoolColumns+` FROM `+spoolsFrom+` WHERE s.id = $1 AND s.deleted_at IS NULL`, id))
		if errors.Is(err, sql.ErrNoRows) {
			return Spool{}, ErrNotFound
		}

		if err != nil {
			return Spool{}, fmt.Errorf("read spool: %w", err)
		}
	}

	manual, nfc := pair[0], pair[1]
	if manual.ArchivedAt != nil || nfc.ArchivedAt != nil {
		return Spool{}, ErrSpoolArchived
	}

	if manual.Source != SourceManual || nfc.Source != SourcePrinter || manual.MaterialUUID != nfc.MaterialUUID {
		return Spool{}, ErrMergeRefused
	}

	if _, err = tx.ExecContext(ctx, `UPDATE spools SET deleted_at = $2, updated_at = $2 WHERE id = $1`,
		from, s.at(now)); err != nil {
		return Spool{}, fmt.Errorf("delete the manual spool: %w", err)
	}

	if err = refuseAssigned(ctx, tx, from); err != nil {
		return Spool{}, err
	}

	if nfc.LocationID == nil {
		nfc.LocationID, nfc.Location = manual.LocationID, manual.Location
	}

	nfc.Notes = cmp.Or(nfc.Notes, manual.Notes)
	nfc.UpdatedAt = now.UTC()

	if _, err = tx.ExecContext(ctx, `UPDATE spools SET location_id = $2, notes = $3, updated_at = $4 WHERE id = $1`,
		into, nfc.LocationID, nfc.Notes, s.at(nfc.UpdatedAt)); err != nil {
		return Spool{}, fmt.Errorf("merge into the printer spool: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return Spool{}, fmt.Errorf("commit: %w", err)
	}

	s.spoolChanged(false, from, into)

	return nfc, nil
}

// SpoolPage is one page of a material's live spools and how many it has.
type SpoolPage struct {
	Spools []Spool `json:"spools"`
	Total  int     `json:"total"`
	Next   *int    `json:"next,omitempty"`
}

// Spools answers one page of the live spools of one material, or of every
// material for "", oldest first.
// A limit of 0 or past MaxPage reads MaxPage.
func (s *Store) Spools(ctx context.Context, materialUUID string, offset, limit int) (SpoolPage, error) {
	if limit <= 0 {
		limit = MaxPage
	}

	limit, offset = min(limit, MaxPage), max(offset, 0)
	page := SpoolPage{Spools: []Spool{}}

	rows, err := s.db.QueryContext(ctx, `SELECT `+spoolColumns+` FROM `+spoolsFrom+`
		WHERE ($1 = '' OR s.material_uuid = $1) AND s.deleted_at IS NULL ORDER BY s.id LIMIT $2 OFFSET $3`,
		materialUUID, limit, offset)
	if err != nil {
		return page, fmt.Errorf("list spools: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		sp, scanErr := scanSpool(rows)
		if scanErr != nil {
			return page, fmt.Errorf("read spool: %w", scanErr)
		}

		page.Spools = append(page.Spools, sp)
	}

	if err = rows.Err(); err != nil {
		return page, fmt.Errorf("list spools: %w", err)
	}

	if err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM spools WHERE ($1 = '' OR material_uuid = $1) AND deleted_at IS NULL`,
		materialUUID).
		Scan(&page.Total); err != nil {
		return page, fmt.Errorf("count spools: %w", err)
	}

	if next := offset + len(page.Spools); next < page.Total {
		page.Next = &next
	}

	return page, nil
}

// keptAt answers the name of the location a spool is put at, and ErrInvalid
// for a location that does not exist.
func keptAt(ctx context.Context, tx *sql.Tx, id int64) (string, error) {
	var name string

	err := tx.QueryRowContext(ctx, `SELECT name FROM locations WHERE id = $1`, id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("there is no location %d: %w", id, ErrInvalid)
	}

	if err != nil {
		return "", fmt.Errorf("read location: %w", err)
	}

	return name, nil
}

// typedFullWeight gives a spool added from a tag with no full weight, whose
// weight a person typed, the full weight of that weight and the grams the
// printer counted used.
func typedFullWeight(ctx context.Context, tx *sql.Tx, sp *Spool) error {
	if sp.Source != SourcePrinter || sp.FullWeightG != nil {
		return nil
	}

	var consumed float64
	if err := tx.QueryRowContext(ctx, `SELECT consumed_weight_g FROM spools WHERE id = $1`, sp.ID).
		Scan(&consumed); err != nil {
		return fmt.Errorf("read the spool's grams used: %w", err)
	}

	sp.FullWeightG = new(sp.RemainingWeightG + consumed)

	return nil
}

// UpdateSpool changes the set fields of a live spool. A spool on a tool takes
// notes, a location and its weight, which the printer counts down from there;
// a change of its archive is ErrSpoolAssigned. A weight typed for a spool
// added from a tag with no full weight also gives it a full weight: the weight
// typed and what the printer counted used.
func (s *Store) UpdateSpool(ctx context.Context, id int64, p SpoolPatch) (Spool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Spool{}, fmt.Errorf("begin: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	sp, err := scanSpool(
		tx.QueryRowContext(
			ctx,
			`SELECT `+spoolColumns+` FROM `+spoolsFrom+` WHERE s.id = $1 AND s.deleted_at IS NULL`,
			id,
		),
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Spool{}, ErrNotFound
	}

	if err != nil {
		return Spool{}, fmt.Errorf("read spool: %w", err)
	}

	if p.RemainingWeightG != nil {
		sp.RemainingWeightG = *p.RemainingWeightG
		if err = typedFullWeight(ctx, tx, &sp); err != nil {
			return Spool{}, err
		}
	}

	if p.Condition != nil {
		sp.Condition = *p.Condition
	}

	switch {
	case p.LocationID == nil:
	case *p.LocationID == 0:
		sp.LocationID, sp.Location = nil, ""
	default:
		if sp.Location, err = keptAt(ctx, tx, *p.LocationID); err != nil {
			return Spool{}, err
		}

		sp.LocationID = p.LocationID
	}

	if p.Notes != nil {
		sp.Notes = *p.Notes
	}

	sp.UpdatedAt = time.Now().UTC()
	patchArchive(&sp, p)

	if !validCondition(sp.Condition) || invalidWeights(sp.RemainingWeightG, sp.FullWeightG, sp.EmptyWeightG) ||
		!fits(sp.Notes, maxNotes) || !validArchive(sp) {
		return Spool{}, ErrInvalid
	}

	if _, err = tx.ExecContext(
		ctx,
		`UPDATE spools SET remaining_weight_g=$2, condition=$3, location_id=$4, notes=$5, updated_at=$6,
			archived_at=$7, archive_reason=NULLIF($8, ''), archive_reason_note=NULLIF($9, ''), full_weight_g=$10
			WHERE id=$1`,
		id,
		sp.RemainingWeightG,
		sp.Condition,
		sp.LocationID,
		sp.Notes,
		s.at(sp.UpdatedAt),
		s.atOrNil(sp.ArchivedAt),
		sp.ArchiveReason,
		sp.ArchiveReasonNote,
		sp.FullWeightG,
	); err != nil {
		return Spool{}, fmt.Errorf("update spool: %w", err)
	}

	if p.touchesStock() {
		if err = refuseAssigned(ctx, tx, id); err != nil {
			return Spool{}, err
		}
	}

	if err = tx.Commit(); err != nil {
		return Spool{}, fmt.Errorf("commit: %w", err)
	}

	s.spoolChanged(false, id)

	return sp, nil
}

// DeleteSpool soft-deletes a live spool. ErrSpoolAssigned for a spool on a tool.
func (s *Store) DeleteSpool(ctx context.Context, id int64, now time.Time) error {
	return s.DeleteSpools(ctx, []int64{id}, now)
}

// DeleteSpools takes 1 to MaxQuantity spools out of stock in one transaction,
// all or none: ErrNotFound when one of the ids is no live spool,
// ErrSpoolAssigned when one is on a tool, ErrInvalid for no id, too many or one
// named twice.
func (s *Store) DeleteSpools(ctx context.Context, ids []int64, now time.Time) error {
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return ErrInvalid
		}

		seen[id] = true
	}

	if len(ids) == 0 || len(ids) > MaxQuantity {
		return ErrInvalid
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	for _, id := range ids {
		res, execErr := tx.ExecContext(ctx,
			`UPDATE spools SET deleted_at=$2, updated_at=$2 WHERE id=$1 AND deleted_at IS NULL`, id, s.at(now))
		if execErr != nil {
			return fmt.Errorf("delete spool %d: %w", id, execErr)
		}

		if n, countErr := res.RowsAffected(); countErr != nil || n == 0 {
			return errors.Join(ErrNotFound, countErr)
		}

		if execErr = refuseAssigned(ctx, tx, id); execErr != nil {
			return execErr
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	s.spoolChanged(false, ids...)

	return nil
}

// copyMaterial fills the spool with the material's fields, a catalogue one or
// the principal's custom one, and the diameter of the package a catalogue
// spool came as. ErrNotFound for a material or package the spool cannot have,
// ErrInvalid for a package named for a custom material.
func copyMaterial(ctx context.Context, tx *sql.Tx, principal string, d SpoolDraft, base *Spool) error {
	err := tx.QueryRowContext(ctx, `SELECT m.name, COALESCE(b.name, ''), m.class, m.type, m.color_rgba, m.density
		FROM materials m LEFT JOIN brands b ON b.slug = m.brand_slug WHERE m.uuid = $1`, d.MaterialUUID).
		Scan(&base.MaterialName, &base.BrandName, &base.Class, &base.Type, &base.ColorRGBA, &base.Density)

	if errors.Is(err, sql.ErrNoRows) {
		base.MaterialSource = "custom"
		err = tx.QueryRowContext(ctx, `SELECT name, brand_name, class, type, color_rgba, density, diameter_um
			FROM custom_materials WHERE uuid = $1 AND owner = $2`, d.MaterialUUID, principal).
			Scan(&base.MaterialName, &base.BrandName, &base.Class, &base.Type, &base.ColorRGBA, &base.Density, &base.DiameterUM)

		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
	}

	if err != nil {
		return fmt.Errorf("find material: %w", err)
	}

	// A custom material comes in no catalogue package.
	if d.PackageUUID != nil && base.MaterialSource == "custom" {
		return fmt.Errorf("a custom material has no package: %w", ErrInvalid)
	}

	// The package gives a catalogue spool its diameter.
	if d.PackageUUID != nil {
		err = tx.QueryRowContext(ctx, `SELECT p.diameter_um FROM packages p JOIN materials m ON m.slug = p.material_slug
			WHERE p.uuid = $1 AND m.uuid = $2`, *d.PackageUUID, d.MaterialUUID).Scan(&base.DiameterUM)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}

		if err != nil {
			return fmt.Errorf("find package: %w", err)
		}
	}

	return nil
}

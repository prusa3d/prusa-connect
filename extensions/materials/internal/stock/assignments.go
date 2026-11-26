package stock

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

var (
	// ErrSpoolOnAnotherTool refuses a spool that is already on another tool.
	ErrSpoolOnAnotherTool = errors.New("the spool is on another tool")
	// ErrSpoolAssigned refuses a change to a spool that is on a tool.
	ErrSpoolAssigned = errors.New("the spool is on a tool")
	// ErrSpoolArchived refuses to put an archived spool on a tool or merge it.
	ErrSpoolArchived = errors.New("the spool is archived")
)

// Who set an assignment.
const (
	ByPerson  = "person"
	ByPrinter = "printer"
)

// Assignment is what is on one printer tool: a material, and a spool of it
// when one was picked. The material fields are copied, as a spool copies them.
type Assignment struct {
	DeviceID       string `json:"device_id"`
	ToolID         string `json:"tool_id"`
	MaterialUUID   string `json:"material_uuid"`
	MaterialSource string `json:"material_source"`
	MaterialName   string `json:"material_name"`
	BrandName      string `json:"brand_name"`
	Type           string `json:"type"`
	Abbreviation   string `json:"abbreviation"`
	ColorRGBA      string `json:"color_rgba"`
	SpoolID        *int64 `json:"spool_id,omitempty"`
	By             string `json:"by"`
	// Reported is the last instance the printer reported, kept for the merge,
	// as EncodeReport writes it.
	Reported  json.RawMessage `json:"-"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// storedReport is a printer's report as an assignment keeps it. Its JSON
// names are the module's own, so the stored rows do not follow the Go field
// names of connectorapi.ReportedMaterial. Its fields are that type's, so the
// two convert into each other.
type storedReport struct {
	InstanceUUID      string             `json:"instance_uuid,omitempty"`
	PackageUUID       string             `json:"package_uuid,omitempty"`
	MaterialUUID      string             `json:"material_uuid,omitempty"`
	Type              string             `json:"type,omitempty"`
	ColorRGBA         string             `json:"color_rgba,omitempty"`
	FullWeight        *float64           `json:"full_weight_g,omitempty"`
	NominalFullWeight *float64           `json:"nominal_full_weight_g,omitempty"`
	Properties        map[string]float64 `json:"properties,omitempty"`
}

// EncodeReport is the JSON an assignment keeps of a printer's report.
func EncodeReport(r connectorapi.ReportedMaterial) (json.RawMessage, error) {
	out, err := json.Marshal(storedReport(r))
	if err != nil {
		return nil, fmt.Errorf("encode the report: %w", err)
	}

	return out, nil
}

// DecodeReport reads the report an assignment keeps. No report reads as an
// empty one.
func DecodeReport(raw json.RawMessage) (connectorapi.ReportedMaterial, error) {
	var r storedReport
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &r); err != nil {
			return connectorapi.ReportedMaterial{}, fmt.Errorf("decode the report: %w", err)
		}
	}

	return connectorapi.ReportedMaterial(r), nil
}

const assignmentColumns = `device_id, tool_id, material_uuid, material_source, material_name, brand_name, type,
	abbreviation, color_rgba, spool_id, assigned_by, reported, updated_at`

// scanAssignment reads the assignment columns, and any columns after them
// into more.
func scanAssignment(row interface{ Scan(...any) error }, more ...any) (Assignment, error) {
	var (
		a        Assignment
		reported sql.NullString
		updated  any
	)

	err := row.Scan(append([]any{&a.DeviceID, &a.ToolID, &a.MaterialUUID, &a.MaterialSource, &a.MaterialName,
		&a.BrandName, &a.Type, &a.Abbreviation, &a.ColorRGBA, &a.SpoolID, &a.By, &reported, &updated}, more...)...)
	if err != nil {
		return a, err
	}

	if reported.Valid {
		a.Reported = json.RawMessage(reported.String)
	}

	a.UpdatedAt, err = parseTime(updated)

	return a, err
}

// Assignments answers what is on each tool of one printer, by tool id.
func (s *Store) Assignments(ctx context.Context, deviceID string) ([]Assignment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+assignmentColumns+` FROM assignments WHERE device_id = $1
		ORDER BY tool_id`, deviceID)
	if err != nil {
		return nil, fmt.Errorf("list assignments: %w", err)
	}

	defer rows.Close()

	out := []Assignment{}

	for rows.Next() {
		a, scanErr := scanAssignment(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("read assignment: %w", scanErr)
		}

		out = append(out, a)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("list assignments: %w", err)
	}

	return out, nil
}

// AssignedDevices answers the printers that hold at least one assignment.
func (s *Store) AssignedDevices(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT device_id FROM assignments ORDER BY device_id`)
	if err != nil {
		return nil, fmt.Errorf("list the assigned printers: %w", err)
	}

	defer rows.Close()

	var out []string

	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read an assigned printer: %w", err)
		}

		out = append(out, id)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("list the assigned printers: %w", err)
	}

	return out, nil
}

// Forget takes every assignment off a printer that no longer exists. Each
// spool goes back to stock, and each tool and spool it freed is told of.
func (s *Store) Forget(ctx context.Context, deviceID string) error {
	held, err := s.Assignments(ctx, deviceID)
	if err != nil {
		return err
	}

	for _, a := range held {
		if err = s.Unassign(ctx, deviceID, a.ToolID); err != nil {
			return err
		}
	}

	return nil
}

// AssignedSpool is an assignment with what is left of the spool on it. The
// spool's fields are empty for an assignment without one.
type AssignedSpool struct {
	Assignment

	SpoolUUID        string
	RemainingWeightG *float64
	FullWeightG      *float64
}

// AssignedSpools answers what is on each tool of the printers, with the spool
// on each, in one query.
func (s *Store) AssignedSpools(ctx context.Context, deviceIDs []string) ([]AssignedSpool, error) {
	if len(deviceIDs) == 0 {
		return []AssignedSpool{}, nil
	}

	args := make([]any, len(deviceIDs))
	for i, id := range deviceIDs {
		args[i] = id
	}

	//nolint:gosec // the placeholders are only $n.
	rows, err := s.db.QueryContext(ctx, `SELECT a.device_id, a.tool_id, a.material_uuid, a.material_source,
		a.material_name, a.brand_name, a.type, a.abbreviation, a.color_rgba, a.spool_id, a.assigned_by, a.reported,
		a.updated_at, sp.uuid, sp.remaining_weight_g, sp.full_weight_g
		FROM assignments a LEFT JOIN spools sp ON sp.id = a.spool_id
		WHERE a.device_id IN (`+placeholders(1, len(args))+`) ORDER BY a.device_id, a.tool_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("list assigned spools: %w", err)
	}

	defer rows.Close()

	out := []AssignedSpool{}

	for rows.Next() {
		var (
			row  AssignedSpool
			uuid sql.NullString
		)

		row.Assignment, err = scanAssignment(rows, &uuid, &row.RemainingWeightG, &row.FullWeightG)
		if err != nil {
			return nil, fmt.Errorf("read assigned spool: %w", err)
		}

		row.SpoolUUID = uuid.String
		out = append(out, row)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("list assigned spools: %w", err)
	}

	return out, nil
}

// Assign puts a material, or a spool, on a tool, in place of what was there.
// A spool gives the material of the spool. Else a person names a catalogue
// material or a custom one of their own, and a printer's assignment keeps
// the fields it was sent with. An assignment sent without Reported keeps the
// report stored on the tool. ErrNotFound for a spool or material that is not
// there, ErrSpoolOnAnotherTool for a spool on another tool.
func (s *Store) Assign(ctx context.Context, principal string, a Assignment) (Assignment, error) {
	return s.AssignIf(ctx, principal, a, nil)
}

// AssignIf assigns as Assign does, once allow passes the assignment with its
// material copied in. An error from allow writes nothing and is answered.
func (s *Store) AssignIf(ctx context.Context, principal string, a Assignment, allow func(Assignment) error,
) (Assignment, error) {
	if a.DeviceID == "" || a.ToolID == "" || (a.By != ByPerson && a.By != ByPrinter) ||
		(a.By == ByPerson && a.SpoolID == nil && a.MaterialUUID == "") {
		return Assignment{}, ErrInvalid
	}

	a, _, err := s.put(ctx, principal, a, false, nil, allow)

	return a, err
}

// Report puts what a printer reported on a tool, by ByPrinter. A spool goes
// with it and leaves any other tool in the same transaction, so an NFC
// report moves a spool. read is the UpdatedAt of the assignment the
// caller read on the tool, and nil for none. A tool that changed since keeps
// what it holds, and Report answers false.
func (s *Store) Report(ctx context.Context, a Assignment, read *time.Time) (bool, error) {
	if a.DeviceID == "" || a.ToolID == "" {
		return false, ErrInvalid
	}

	a.By = ByPrinter
	_, written, err := s.put(ctx, "", a, true, &read, nil)

	return written, err
}

// The update of a tool that holds an assignment already. The tool keeps its
// last odometer reading: what it feeds after it is charged to
// the spool on it when the next reading comes.
const replaceTool = `DO UPDATE SET material_uuid = excluded.material_uuid,
	material_source = excluded.material_source, material_name = excluded.material_name,
	brand_name = excluded.brand_name, type = excluded.type, abbreviation = excluded.abbreviation,
	color_rgba = excluded.color_rgba, spool_id = excluded.spool_id, assigned_by = excluded.assigned_by,
	reported = COALESCE(excluded.reported, assignments.reported), updated_at = excluded.updated_at`

// put writes one assignment in one transaction. move takes its spool off any
// other tool first. guard nil writes whatever the tool holds. Else *guard is
// the UpdatedAt the tool must still hold, or nil for a tool that must still
// be empty, and written is false when it does not. allow, when set, sees the
// assignment with its material copied in before anything is written.
func (s *Store) put(ctx context.Context, principal string, a Assignment, move bool, guard **time.Time,
	allow func(Assignment) error,
) (Assignment, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Assignment{}, false, fmt.Errorf("begin: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	switch {
	case a.SpoolID != nil:
		err = copySpool(ctx, tx, &a)
		if err == nil {
			err = s.unarchived(ctx, tx, *a.SpoolID, a.By)
		}
	case a.By == ByPerson:
		err = copyAssigned(ctx, tx, principal, &a)
	}

	if err == nil && allow != nil {
		err = allow(a)
	}

	if err != nil {
		return Assignment{}, false, err
	}

	gone, err := takeAway(ctx, tx, a, move)
	if err != nil {
		return Assignment{}, false, err
	}

	var reported any
	if a.Reported != nil {
		reported = string(a.Reported)
	}

	a.UpdatedAt = time.Now().UTC()
	args := []any{
		a.DeviceID, a.ToolID, a.MaterialUUID, a.MaterialSource, a.MaterialName, a.BrandName, a.Type,
		a.Abbreviation, a.ColorRGBA, a.SpoolID, a.By, reported, s.at(a.UpdatedAt),
	}

	conflict := replaceTool

	switch {
	case guard != nil && *guard == nil:
		conflict = `DO NOTHING`
	case guard != nil:
		conflict += ` WHERE assignments.updated_at = $14`
		args = append(args, s.at(**guard))
	}

	res, err := tx.ExecContext(ctx, `INSERT INTO assignments (`+assignmentColumns+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (device_id, tool_id) `+conflict, args...)
	if unique(err) {
		return Assignment{}, false, ErrSpoolOnAnotherTool
	}

	if err != nil {
		return Assignment{}, false, fmt.Errorf("assign: %w", err)
	}

	if n, countErr := res.RowsAffected(); countErr != nil || n == 0 {
		// The tool changed since it was read. The move is undone with it.
		return Assignment{}, false, countErr
	}

	if err = tx.Commit(); err != nil {
		return Assignment{}, false, fmt.Errorf("commit: %w", err)
	}

	s.toolChanged(a.DeviceID, a.ToolID)

	if gone.deviceID != "" {
		s.toolChanged(gone.deviceID, gone.toolID)
	}

	for _, id := range []*int64{a.SpoolID, gone.spoolID} {
		if id != nil {
			s.spoolChanged(false, *id)
		}
	}

	return a, true, nil
}

// taken is what a write of a tool takes away: the spool the tool held, and
// the tool a moved spool leaves.
type taken struct {
	spoolID          *int64
	deviceID, toolID string
}

// takeAway reads the spool the tool holds, and with move takes the spool off
// any other tool, so the write can tell of both.
func takeAway(ctx context.Context, tx *sql.Tx, a Assignment, move bool) (taken, error) {
	var gone taken

	err := tx.QueryRowContext(ctx, `SELECT spool_id FROM assignments WHERE device_id = $1 AND tool_id = $2`,
		a.DeviceID, a.ToolID).Scan(&gone.spoolID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return gone, fmt.Errorf("read the tool: %w", err)
	}

	if !move || a.SpoolID == nil {
		return gone, nil
	}

	err = tx.QueryRowContext(ctx, `DELETE FROM assignments WHERE spool_id = $1
		AND NOT (device_id = $2 AND tool_id = $3) RETURNING device_id, tool_id`,
		*a.SpoolID, a.DeviceID, a.ToolID).Scan(&gone.deviceID, &gone.toolID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return gone, fmt.Errorf("move the spool: %w", err)
	}

	return gone, nil
}

// copySpool fills the assignment with the material of its live spool.
//
// It writes the spool row first, which changes nothing but locks the row on
// Postgres. A spool edit or delete writes the row too, and then looks for an
// assignment. So the two wait for each other, and the later one sees what the
// other did.
func copySpool(ctx context.Context, tx *sql.Tx, a *Assignment) error {
	res, err := tx.ExecContext(ctx, `UPDATE spools SET updated_at = updated_at WHERE id = $1 AND deleted_at IS NULL`,
		*a.SpoolID)
	if err != nil {
		return fmt.Errorf("lock spool: %w", err)
	}

	if n, countErr := res.RowsAffected(); countErr != nil || n == 0 {
		return errors.Join(ErrNotFound, countErr)
	}

	// A spool of a material neither table holds keeps the abbreviation the caller sent.
	err = tx.QueryRowContext(ctx, `SELECT s.material_uuid, s.material_source, s.material_name, s.brand_name, s.type,
		COALESCE(NULLIF(m.abbreviation, ''), NULLIF(c.abbreviation, ''), $2), s.color_rgba
		FROM spools s LEFT JOIN materials m ON m.uuid = s.material_uuid
		LEFT JOIN custom_materials c ON c.uuid = s.material_uuid WHERE s.id = $1`, *a.SpoolID, a.Abbreviation).
		Scan(&a.MaterialUUID, &a.MaterialSource, &a.MaterialName, &a.BrandName, &a.Type, &a.Abbreviation, &a.ColorRGBA)
	if err != nil {
		return fmt.Errorf("read spool: %w", err)
	}

	return nil
}

// unarchived refuses an archived spool a person puts on a tool, and brings
// one back to stock that a printer reads off its tag.
func (s *Store) unarchived(ctx context.Context, tx *sql.Tx, spoolID int64, by string) error {
	if by == ByPrinter {
		_, err := s.restore(ctx, tx, spoolID)

		return err
	}

	var archived bool
	if err := tx.QueryRowContext(ctx, `SELECT archived_at IS NOT NULL FROM spools WHERE id = $1`, spoolID).
		Scan(&archived); err != nil {
		return fmt.Errorf("read spool: %w", err)
	}

	if archived {
		return ErrSpoolArchived
	}

	return nil
}

// copyAssigned fills the assignment with a catalogue material, or with a
// custom material the principal owns.
func copyAssigned(ctx context.Context, tx *sql.Tx, principal string, a *Assignment) error {
	a.MaterialSource = "catalogue"
	err := tx.QueryRowContext(ctx, `SELECT m.name, COALESCE(b.name, ''), m.type, m.abbreviation, m.color_rgba
		FROM materials m LEFT JOIN brands b ON b.slug = m.brand_slug WHERE m.uuid = $1`, a.MaterialUUID).
		Scan(&a.MaterialName, &a.BrandName, &a.Type, &a.Abbreviation, &a.ColorRGBA)

	if errors.Is(err, sql.ErrNoRows) {
		a.MaterialSource = "custom"
		err = tx.QueryRowContext(ctx, `SELECT name, brand_name, type, abbreviation, color_rgba
			FROM custom_materials WHERE uuid = $1 AND owner = $2`, a.MaterialUUID, principal).
			Scan(&a.MaterialName, &a.BrandName, &a.Type, &a.Abbreviation, &a.ColorRGBA)
	}

	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}

	if err != nil {
		return fmt.Errorf("find material: %w", err)
	}

	return nil
}

// Refresh writes the report and the material fields of an assignment that
// stands, and leaves who set it and its spool as they are. a.UpdatedAt is
// what the caller read: a tool that changed since, or holds nothing, keeps
// what it holds.
func (s *Store) Refresh(ctx context.Context, a Assignment) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE assignments SET material_uuid = $1, type = $2, abbreviation = $3,
		color_rgba = $4, reported = $5, updated_at = $6 WHERE device_id = $7 AND tool_id = $8 AND updated_at = $9`,
		a.MaterialUUID, a.Type, a.Abbreviation, a.ColorRGBA, string(a.Reported), s.at(time.Now().UTC()),
		a.DeviceID, a.ToolID, s.at(a.UpdatedAt)); err != nil {
		return fmt.Errorf("refresh assignment: %w", err)
	}

	s.toolChanged(a.DeviceID, a.ToolID)

	return nil
}

// Drop clears a tool the printer reported empty or took away. a.UpdatedAt is
// what the caller read: a tool that changed since keeps what it holds.
func (s *Store) Drop(ctx context.Context, a Assignment) error {
	return s.unassign(ctx, a.DeviceID, a.ToolID, `AND updated_at = $3`, s.at(a.UpdatedAt))
}

// unassign deletes one tool's assignment, held to more, and tells of the tool
// and the spool it freed.
func (s *Store) unassign(ctx context.Context, deviceID, toolID, more string, args ...any) error {
	var held *int64

	err := s.db.QueryRowContext(ctx, `DELETE FROM assignments WHERE device_id = $1 AND tool_id = $2 `+more+`
		RETURNING spool_id`, append([]any{deviceID, toolID}, args...)...).Scan(&held)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("unassign: %w", err)
	}

	s.toolChanged(deviceID, toolID)

	if held != nil {
		s.spoolChanged(false, *held)
	}

	return nil
}

// SpoolByUUID answers the id of the live spool read off a tag with that uuid,
// and ok false for none.
func (s *Store) SpoolByUUID(ctx context.Context, uuid string) (id int64, ok bool, err error) {
	// A manual spool's uuid is the module's own, shown to anyone who reads the
	// stock, so a printer reporting it names no spool.
	err = s.db.QueryRowContext(ctx, `SELECT id FROM spools WHERE uuid = $1 AND source = $2 AND deleted_at IS NULL`,
		uuid, SourcePrinter).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}

	if err != nil {
		return 0, false, fmt.Errorf("find the spool: %w", err)
	}

	return id, true, nil
}

// Unassign clears a tool. A tool with nothing on it is no error.
func (s *Store) Unassign(ctx context.Context, deviceID, toolID string) error {
	return s.unassign(ctx, deviceID, toolID, "")
}

// refuseAssigned answers ErrSpoolAssigned for a spool on a tool. A caller
// writes the spool row first, so an assign of it waits until the caller is
// done, as copySpool says.
func refuseAssigned(ctx context.Context, tx *sql.Tx, spoolID int64) error {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM assignments WHERE spool_id = $1`, spoolID).
		Scan(&n); err != nil {
		return fmt.Errorf("find the spool's tool: %w", err)
	}

	if n > 0 {
		return ErrSpoolAssigned
	}

	return nil
}

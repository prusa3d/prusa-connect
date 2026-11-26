package stock

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"unicode/utf8"
)

// archiveReasons are the reasons a spool is archived for.
var archiveReasons = map[string]bool{
	"consumed": true, "defective": true, "lost": true, "expired": true, "damaged": true, "other": true,
}

// maxArchiveNote is the longest archive note, in characters.
const maxArchiveNote = 1000

// validArchive refuses a reason archiveReasons does not hold, a note too long, or
// either on a spool in stock.
func validArchive(sp Spool) bool {
	if sp.ArchivedAt == nil {
		return sp.ArchiveReason == "" && sp.ArchiveReasonNote == ""
	}

	return (sp.ArchiveReason == "" || archiveReasons[sp.ArchiveReason]) &&
		utf8.RuneCountInString(
			sp.ArchiveReasonNote,
		) <= maxArchiveNote && fits(sp.ArchiveReasonNote, utf8.UTFMax*maxArchiveNote)
}

// restore brings an archived spool back to stock, clearing its reason and
// note, and answers whether it was archived.
func (s *Store) restore(ctx context.Context, tx *sql.Tx, id int64) (bool, error) {
	res, err := tx.ExecContext(ctx, `UPDATE spools SET archived_at = NULL, archive_reason = NULL,
		archive_reason_note = NULL, updated_at = $2 WHERE id = $1 AND archived_at IS NOT NULL`,
		id, s.at(time.Now().UTC()))
	if err != nil {
		return false, fmt.Errorf("restore spool: %w", err)
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// patchArchive archives or restores sp as p says, then sets its reason and
// note. Archiving an archived spool keeps when it was, and restoring clears
// the reason and the note.
func patchArchive(sp *Spool, p SpoolPatch) {
	switch {
	case p.Archived == nil:
	case *p.Archived && sp.ArchivedAt == nil:
		sp.ArchivedAt = &sp.UpdatedAt
	case !*p.Archived:
		sp.ArchivedAt, sp.ArchiveReason, sp.ArchiveReasonNote = nil, "", ""
	}

	if p.ArchiveReason != nil {
		sp.ArchiveReason = *p.ArchiveReason
	}

	if p.ArchiveReasonNote != nil {
		sp.ArchiveReasonNote = *p.ArchiveReasonNote
	}
}

// touchesStock is true for a patch of what a spool on a tool may not change:
// its archive, as an archived spool goes on no tool. Its weight a person may
// correct, and the printer counts down from there.
func (p SpoolPatch) touchesStock() bool {
	return p.Condition != nil || p.Archived != nil || p.ArchiveReason != nil || p.ArchiveReasonNote != nil
}

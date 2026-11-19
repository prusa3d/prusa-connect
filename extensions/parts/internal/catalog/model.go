package catalog

import (
	"errors"
	"time"
)

// Format is the format version an earlier release wrote into every record of
// the record store, and the one every row reads as now. A record of another
// format is not moved rather than read into the wrong fields.
const Format = 1

// Root is the folder id a top-level folder and a top-level part name as their
// parent. No row carries it as its own id.
const Root = 0

// The errors the rules in this package return. A caller branches on them with
// errors.Is and turns each into the code a client reads. Each message names the
// field and the limit, because that is what the person has to change.
var (
	// ErrInvalid reports a value a person cannot have meant.
	ErrInvalid = errors.New("parts: the request was not accepted")

	// ErrFolderNotFound reports a folder id the catalogue does not hold.
	ErrFolderNotFound = errors.New("parts: the catalogue holds no folder with that id")

	// ErrPartNotFound reports a part id the catalogue does not hold.
	ErrPartNotFound = errors.New("parts: the catalogue holds no part with that id")

	// ErrVersionNotFound reports a version number the part does not have.
	ErrVersionNotFound = errors.New("parts: the part has no version with that number")

	// ErrCommentNotFound reports a comment id the part does not have.
	ErrCommentNotFound = errors.New("parts: the part has no comment with that id")

	// ErrNameTaken reports a folder name its parent already holds.
	ErrNameTaken = errors.New("parts: the folder already holds a folder with that name")

	// ErrFolderNotEmpty reports a folder that still holds a folder or a part.
	ErrFolderNotEmpty = errors.New("parts: the folder still holds something")

	// ErrNotAuthor reports a change to a comment somebody else wrote.
	ErrNotAuthor = errors.New("parts: only the person who wrote a comment may change it")

	// ErrFileMissing reports a file the storage does not hold, or one the
	// person asking may not read.
	ErrFileMissing = errors.New("parts: the storage holds no such file")

	// ErrFull reports a write over a limit: how many of one thing a part may
	// carry.
	ErrFull = errors.New("parts: the catalogue already holds as much as it may")

	// ErrConflict reports a write against a record somebody else changed
	// first. The caller reads it again and decides again.
	ErrConflict = errors.New("parts: the record changed while this request was being served")

	// ErrStore reports a database, a record store or a storage that could
	// not answer. Nothing the caller sent is at fault.
	ErrStore = errors.New("parts: the catalogue's store could not answer")
)

// Folder is one folder of the tree. A folder holds folders and parts, and the
// tree starts at Root.
type Folder struct {
	// V is the format of this row.
	V int `json:"v"`
	// ID is the number the module counted up. It is never given out twice.
	ID int `json:"id"`
	// Parent is the folder this one is in, and Root for a top-level folder.
	Parent int `json:"parent"`
	// Name is what the tree shows. Two folders in one parent never share one.
	Name string `json:"name"`
	// Description is a few lines a person writes about what the folder holds.
	Description string `json:"description,omitempty"`

	Created   time.Time `json:"created"`
	Changed   time.Time `json:"changed"`
	ChangedBy string    `json:"changed_by"`
}

// Part is one entry of the catalogue: a thing a person makes, with the words
// that describe it. The files behind it are its versions, kept as rows of
// their own.
type Part struct {
	// V is the format of this row.
	V int `json:"v"`
	// ID is the number the module counted up. It is never given out twice,
	// so a link to a deleted part never opens another one.
	ID int `json:"id"`
	// Folder is the folder the part is in, and Root for one at the top.
	Folder int `json:"folder"`
	// Name is what a listing shows.
	Name string `json:"name"`
	// Summary is the one line under the name.
	Summary string `json:"summary,omitempty"`
	// Description is the long text a person edits: what the part is, how it
	// is printed, what it fits.
	Description string `json:"description,omitempty"`
	// Tags are lower-case words a person filters the catalogue by.
	Tags []string `json:"tags,omitempty"`
	// Cover is the path of one of the part's files that stands for the part
	// in a listing. Empty lets the module choose.
	Cover string `json:"cover,omitempty"`
	// LastVersion is the highest version number the part ever gave out, so
	// a number is never given out twice, also after the newest version was
	// deleted. A record of an earlier release written before it existed
	// reads zero, and the move takes the highest version it holds instead.
	LastVersion int `json:"last_version,omitempty"`

	Created   time.Time `json:"created"`
	CreatedBy string    `json:"created_by"`
	Changed   time.Time `json:"changed"`
	ChangedBy string    `json:"changed_by"`
}

// File is one file of a version, in the storage the page uploaded it to.
type File struct {
	// Name is what a person reads: the last segment of the path.
	Name string `json:"name"`
	// Storage is the storage the file is in, by the name it is declared under.
	Storage string `json:"storage"`
	// Path is the file inside that storage.
	Path string `json:"path"`
	// Size is the length the storage reported when the version was written.
	Size int64 `json:"size"`
	// SHA256 is the digest the storage reported, and empty where it keeps none.
	SHA256 string `json:"sha256,omitempty"`
	// Thumbnail is whether the storage reported a picture inside the file when
	// the version was written. It is nil where the storage did not say, which
	// includes every file written before it was recorded, so a page still asks
	// for those.
	Thumbnail *bool `json:"thumbnail,omitempty"`
}

// Version is one state of a part: the files it was at, and a note on what
// changed. The files of a version do not change. A new set is a new version.
type Version struct {
	// V is the format of this row.
	V int `json:"v"`
	// Part is the part this version belongs to.
	Part int `json:"part"`
	// Number counts the versions of one part from 1.
	Number int `json:"number"`
	// Note is what a person wrote about this version.
	Note string `json:"note,omitempty"`
	// Files are the files of this version. One file may be in several
	// versions, which is how a version keeps what the one before it had.
	Files []File `json:"files"`

	Created   time.Time `json:"created"`
	CreatedBy string    `json:"created_by"`
	Changed   time.Time `json:"changed"`
	ChangedBy string    `json:"changed_by"`
}

// Comment is one note a person left on a part.
type Comment struct {
	// V is the format of this row.
	V int `json:"v"`
	// ID is the number the module counted up, across every part.
	ID int `json:"id"`
	// Part is the part the comment is on.
	Part int `json:"part"`
	// Body is what the person wrote.
	Body string `json:"body"`
	// By is the person the host named when the comment was written. Only that
	// person may change it.
	By string `json:"by"`

	Created time.Time `json:"created"`
	// Edited is when the body was last changed, and absent for one never
	// changed.
	Edited *time.Time `json:"edited,omitempty"`
}

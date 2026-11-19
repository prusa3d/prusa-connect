package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/storageapi"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"

	"github.com/prusa3d/parts/internal/catalog"
	"github.com/prusa3d/parts/internal/dbtest"
	"github.com/prusa3d/parts/internal/hostfake"
)

// The place every test files its uploads under.
var place = catalog.Place{Storage: "local", Root: "parts"}

func at() time.Time { return time.Date(2026, time.October, 5, 9, 0, 0, 0, time.UTC) }

// fresh builds an empty catalogue over a new SQLite database and a storage in
// memory, and reads it, which is what the first request does.
func fresh(t *testing.T) (*catalog.Catalog, *hostfake.Files) {
	t.Helper()

	grant := dbtest.Engines(t)[0].Grant(t)
	held, files := over(t, grant, &hostfake.Records{}, &hostfake.Files{})

	return held, files
}

// over builds a catalogue over the database a grant names, which is one
// start of the module, and reads it.
func over(
	t *testing.T, grant wirerpc.DatabaseGrant, records catalog.Records, files *hostfake.Files,
) (*catalog.Catalog, *hostfake.Files) {
	t.Helper()

	held := catalog.New(records, files, place, at)
	held.Attach(dbtest.Open(t, grant), grant.Engine)

	if err := held.Ensure(context.Background()); err != nil {
		t.Fatalf("read the database: %v", err)
	}

	return held, files
}

// aPart adds one part with the name given, at the top of the tree.
func aPart(t *testing.T, held *catalog.Catalog, name string) catalog.Part {
	t.Helper()

	part, err := held.AddPart(context.Background(), catalog.PartDraft{Name: new(name)}, "user:alice")
	if err != nil {
		t.Fatalf("add %s: %v", name, err)
	}

	return part
}

// aVersion uploads the files given into the part's folder and records them as
// one version.
func aVersion(t *testing.T, held *catalog.Catalog, files *hostfake.Files, part int, names ...string) catalog.Version {
	t.Helper()

	drafts := make([]catalog.FileDraft, 0, len(names))

	for _, name := range names {
		path := catalog.PartDir(place.Root, part) + "/up/" + name
		files.Put(place.Storage, path, int64(len(name)), "digest-"+name)
		drafts = append(drafts, catalog.FileDraft{Path: path})
	}

	version, err := held.AddVersion(context.Background(), part, catalog.VersionDraft{Files: drafts}, "user:alice")
	if err != nil {
		t.Fatalf("add a version of part %d: %v", part, err)
	}

	return version
}

func TestTheCatalogueSurvivesARestart(t *testing.T) {
	t.Parallel()

	for _, engine := range dbtest.Engines(t) {
		t.Run(string(engine.Name), func(t *testing.T) {
			t.Parallel()

			grant := engine.Grant(t)
			held, files := over(t, grant, nil, &hostfake.Files{})
			ctx := context.Background()

			folder, err := held.AddFolder(ctx, catalog.FolderDraft{Name: new("Brackets")}, "user:alice")
			if err != nil {
				t.Fatalf("add a folder: %v", err)
			}

			part, err := held.AddPart(ctx, catalog.PartDraft{
				Name: new("Shelf bracket"), Folder: new(folder.ID), Tags: &[]string{"Shelf", "petg", "shelf"},
			}, "user:alice")
			if err != nil {
				t.Fatalf("add a part: %v", err)
			}

			aVersion(t, held, files, part.ID, "bracket.3mf", "bracket.bgcode")

			comment, err := held.AddComment(ctx, part.ID, "prints well", "user:bob")
			if err != nil {
				t.Fatalf("add a comment: %v", err)
			}

			if _, err = held.ChangeComment(ctx, part.ID, comment.ID, "prints well in PETG", "user:bob"); err != nil {
				t.Fatalf("change the comment: %v", err)
			}

			// A second process over the same database, which is a restart.
			again, _ := over(t, grant, nil, files)

			detail, err := again.Detail(part.ID)
			if err != nil {
				t.Fatalf("the part is gone after a restart: %v", err)
			}

			if detail.Folder != folder.ID || len(detail.Versions) != 1 || len(detail.Comments) != 1 {
				t.Fatalf("the part came back as %+v", detail)
			}

			if got := strings.Join(detail.Tags, ","); got != "petg,shelf" {
				t.Errorf("the tags read %q, and they are lower-cased, sorted and kept once", got)
			}

			if files := detail.Versions[0].Files; len(files) != 2 || files[0].Name != "bracket.3mf" ||
				files[1].SHA256 != "digest-bracket.bgcode" {
				t.Errorf("the files of the version came back as %+v", files)
			}

			if got := detail.Comments[0]; got.Body != "prints well in PETG" || got.Edited == nil ||
				!got.Created.Equal(at()) {
				t.Errorf("the comment came back as %+v", got)
			}

			if detail.Picture == nil || detail.Picture.Kind != catalog.PictureThumbnail {
				t.Errorf("a part with a print file draws its thumbnail, and it drew %+v", detail.Picture)
			}

			// An id is never given out twice, also after a restart.
			next := aPart(t, again, "Hinge")
			if next.ID == part.ID {
				t.Errorf("the part added after the restart took id %d, which was already given out", next.ID)
			}
		})
	}
}

func TestAFolderCannotGoInsideItself(t *testing.T) {
	t.Parallel()

	held, _ := fresh(t)
	ctx := context.Background()

	outer, _ := held.AddFolder(ctx, catalog.FolderDraft{Name: new("Outer")}, "user:alice")
	inner, _ := held.AddFolder(ctx, catalog.FolderDraft{Name: new("Inner"), Parent: new(outer.ID)}, "user:alice")

	if _, err := held.ChangeFolder(
		ctx,
		outer.ID,
		catalog.FolderDraft{Parent: new(inner.ID)},
		"user:alice",
	); !errors.Is(
		err,
		catalog.ErrInvalid,
	) {
		t.Errorf("moving a folder into its own child answered %v", err)
	}

	if _, err := held.ChangeFolder(
		ctx,
		outer.ID,
		catalog.FolderDraft{Parent: new(outer.ID)},
		"user:alice",
	); !errors.Is(
		err,
		catalog.ErrInvalid,
	) {
		t.Errorf("moving a folder into itself answered %v", err)
	}
}

func TestTwoFoldersInOneParentNeverShareAName(t *testing.T) {
	t.Parallel()

	held, _ := fresh(t)
	ctx := context.Background()

	if _, err := held.AddFolder(ctx, catalog.FolderDraft{Name: new("Jigs")}, "user:alice"); err != nil {
		t.Fatalf("add a folder: %v", err)
	}

	if _, err := held.AddFolder(
		ctx,
		catalog.FolderDraft{Name: new("jigs")},
		"user:alice",
	); !errors.Is(
		err,
		catalog.ErrNameTaken,
	) {
		t.Errorf("a second folder named jigs answered %v", err)
	}

	if _, err := held.AddFolder(
		ctx,
		catalog.FolderDraft{Name: new("a/b")},
		"user:alice",
	); !errors.Is(
		err,
		catalog.ErrInvalid,
	) {
		t.Errorf("a folder name with a slash answered %v", err)
	}
}

func TestAFolderWithSomethingInItIsNotDeleted(t *testing.T) {
	t.Parallel()

	held, _ := fresh(t)
	ctx := context.Background()

	folder, _ := held.AddFolder(ctx, catalog.FolderDraft{Name: new("Clips")}, "user:alice")
	part, _ := held.AddPart(ctx, catalog.PartDraft{Name: new("Clip"), Folder: new(folder.ID)}, "user:alice")

	if err := held.DeleteFolder(ctx, folder.ID); !errors.Is(err, catalog.ErrFolderNotEmpty) {
		t.Fatalf("deleting a folder with a part in it answered %v", err)
	}

	if _, err := held.DeletePart(ctx, part.ID); err != nil {
		t.Fatalf("delete the part: %v", err)
	}

	if err := held.DeleteFolder(ctx, folder.ID); err != nil {
		t.Errorf("deleting the empty folder answered %v", err)
	}
}

func TestAVersionNamesOnlyFilesInItsOwnPartsFolder(t *testing.T) {
	t.Parallel()

	held, files := fresh(t)
	ctx := context.Background()

	first := aPart(t, held, "First")
	second := aPart(t, held, "Second")

	files.Put("local", "parts/2/up/other.stl", 10, "")
	files.Put("local", "elsewhere/up/loose.stl", 10, "")

	for _, written := range []string{
		"parts/2/up/other.stl",         // another part's file
		"elsewhere/up/loose.stl",       // outside the catalogue
		"parts/1/../2/up/other.stl",    // a climb
		"parts/1/up/nested/deeper.stl", // two segments after the upload
		"parts/1/Up Load/name.stl",     // an upload segment that is not one
		"parts/1/up/",                  // no file name
	} {
		_, err := held.AddVersion(ctx, first.ID, catalog.VersionDraft{
			Files: []catalog.FileDraft{{Path: written}},
		}, "user:alice")
		if !errors.Is(err, catalog.ErrInvalid) {
			t.Errorf("a version of part %d naming %q answered %v", first.ID, written, err)
		}
	}

	// A file in the right place that the storage does not hold.
	_, err := held.AddVersion(ctx, first.ID, catalog.VersionDraft{
		Files: []catalog.FileDraft{{Path: "parts/1/up/never-uploaded.stl"}},
	}, "user:alice")
	if !errors.Is(err, catalog.ErrFileMissing) {
		t.Errorf("a file the storage does not hold answered %v", err)
	}

	_ = second
}

func TestTheStorageIsAskedAsThePerson(t *testing.T) {
	t.Parallel()

	held, files := fresh(t)
	part := aPart(t, held, "Bracket")

	files.Fail = func(params storageapi.StatParams) error {
		if params.Principal != "user:alice" {
			return storageapi.ErrDenied
		}

		return nil
	}

	aVersion(t, held, files, part.ID, "bracket.stl")

	for _, asked := range files.Asked {
		if asked != "user:alice" {
			t.Errorf("the storage was asked as %q, and the request was alice's", asked)
		}
	}
}

func TestDeletingAVersionKeepsTheFilesAnotherVersionNames(t *testing.T) {
	t.Parallel()

	held, files := fresh(t)
	ctx := context.Background()
	part := aPart(t, held, "Bracket")

	first := aVersion(t, held, files, part.ID, "bracket.3mf", "notes.pdf")

	// The second version keeps the drawing and replaces the model.
	files.Put("local", "parts/1/two/bracket.3mf", 20, "")

	second, err := held.AddVersion(ctx, part.ID, catalog.VersionDraft{Files: []catalog.FileDraft{
		{Path: "parts/1/two/bracket.3mf"},
		{Storage: "local", Path: first.Files[1].Path},
	}}, "user:alice")
	if err != nil {
		t.Fatalf("add the second version: %v", err)
	}

	if second.Number != 2 {
		t.Fatalf("the second version is number %d", second.Number)
	}

	removed, err := held.DeleteVersion(ctx, part.ID, first.Number)
	if err != nil {
		t.Fatalf("delete the first version: %v", err)
	}

	if len(removed.Files) != 1 || removed.Files[0].Path != first.Files[0].Path {
		t.Errorf("deleting version 1 answered %+v, and only its model is named by nothing now", removed.Files)
	}
}

func TestOnlyTheAuthorChangesAComment(t *testing.T) {
	t.Parallel()

	held, _ := fresh(t)
	ctx := context.Background()
	part := aPart(t, held, "Bracket")

	comment, err := held.AddComment(ctx, part.ID, "first try failed at layer 3", "user:bob")
	if err != nil {
		t.Fatalf("add a comment: %v", err)
	}

	if _, err = held.ChangeComment(
		ctx,
		part.ID,
		comment.ID,
		"rewritten",
		"user:alice",
	); !errors.Is(
		err,
		catalog.ErrNotAuthor,
	) {
		t.Errorf("alice rewriting bob's comment answered %v", err)
	}

	if err = held.DeleteComment(ctx, part.ID, comment.ID, "user:alice"); !errors.Is(err, catalog.ErrNotAuthor) {
		t.Errorf("alice deleting bob's comment answered %v", err)
	}

	changed, err := held.ChangeComment(ctx, part.ID, comment.ID, "fixed with a brim", "user:bob")
	if err != nil || changed.Edited == nil {
		t.Fatalf("bob rewriting his own comment answered %+v, %v", changed, err)
	}

	if _, err = held.AddComment(ctx, part.ID, "   ", "user:bob"); !errors.Is(err, catalog.ErrInvalid) {
		t.Errorf("an empty comment answered %v", err)
	}
}

func TestAChangeMadeAgainstAnOldRevisionIsRefused(t *testing.T) {
	t.Parallel()

	held, _ := fresh(t)
	ctx := context.Background()
	part := aPart(t, held, "Bracket")

	read, _ := held.Detail(part.ID)
	old := read.Revision

	if _, err := held.ChangePart(ctx, part.ID, catalog.PartDraft{
		Description: new("alice's text"), Revision: &old,
	}, "user:alice"); err != nil {
		t.Fatalf("the first change answered %v", err)
	}

	if _, err := held.ChangePart(ctx, part.ID, catalog.PartDraft{
		Description: new("bob's text"), Revision: &old,
	}, "user:bob"); !errors.Is(err, catalog.ErrConflict) {
		t.Errorf("a change against the revision alice already moved answered %v", err)
	}

	now, _ := held.Detail(part.ID)
	if now.Description != "alice's text" {
		t.Errorf("the description reads %q, and bob's change was refused", now.Description)
	}
}

// A second copy of the module over the same database changes a part. The
// first copy's change is then made against a revision the database no longer
// holds, and it is refused rather than written over the other change.
func TestAWriteAgainstARevisionTheDatabaseMovedIsRefused(t *testing.T) {
	t.Parallel()

	grant := dbtest.Engines(t)[0].Grant(t)
	held, files := over(t, grant, nil, &hostfake.Files{})
	ctx := context.Background()
	part := aPart(t, held, "Bracket")

	other, _ := over(t, grant, nil, files)
	if _, err := other.ChangePart(ctx, part.ID, catalog.PartDraft{Name: new("Moved")}, "user:bob"); err != nil {
		t.Fatalf("the other copy's change answered %v", err)
	}

	if _, err := held.ChangePart(
		ctx,
		part.ID,
		catalog.PartDraft{Name: new("Renamed")},
		"user:alice",
	); !errors.Is(
		err,
		catalog.ErrConflict,
	) {
		t.Fatalf("a write against a moved revision answered %v", err)
	}

	if now, _ := held.Detail(part.ID); now.Name != "Bracket" {
		t.Errorf("the copy reads %q after a refused write", now.Name)
	}
}

// The page reads the tags of every part it opens, so a part with none sends an
// empty list rather than leaving the field out.
func TestAPartWithNoTagsSendsAnEmptyList(t *testing.T) {
	t.Parallel()

	held, _ := fresh(t)
	part := aPart(t, held, "Shelf bracket")

	detail, err := held.Detail(part.ID)
	if err != nil {
		t.Fatalf("read the part: %v", err)
	}

	body, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("encode the part: %v", err)
	}

	if !strings.Contains(string(body), `"tags":[]`) {
		t.Errorf("a part with no tags was sent as %s", body)
	}
}

func TestSearchFindsWordsTagsAndFileNames(t *testing.T) {
	t.Parallel()

	held, files := fresh(t)
	ctx := context.Background()

	folder, _ := held.AddFolder(ctx, catalog.FolderDraft{Name: new("Garage")}, "user:alice")
	inner, _ := held.AddFolder(ctx, catalog.FolderDraft{Name: new("Shelves"), Parent: new(folder.ID)}, "user:alice")

	hook, _ := held.AddPart(ctx, catalog.PartDraft{
		Name: new("Hook"), Folder: new(inner.ID), Description: new("Holds a bicycle by its wheel"),
	}, "user:alice")
	aVersion(t, held, files, hook.ID, "wheel-hook-v2.bgcode")

	_, _ = held.AddPart(ctx, catalog.PartDraft{Name: new("Knob"), Tags: &[]string{"kitchen"}}, "user:alice")

	for query, wanted := range map[string]int{
		"BICYCLE": 1, // the description, ignoring case
		"hook-v2": 1, // a file name
		"kitchen": 1, // a tag
		"nothing": 0,
		"":        2,
	} {
		if got := held.List(catalog.Query{Text: query}).Matched; got != wanted {
			t.Errorf("searching %q matched %d parts and %d hold it", query, got, wanted)
		}
	}

	top := folder.ID
	if got := held.List(catalog.Query{Folder: &top}).Matched; got != 0 {
		t.Errorf("the garage holds no part directly, and the listing matched %d", got)
	}

	if got := held.List(catalog.Query{Folder: &top, Deep: true}).Matched; got != 1 {
		t.Errorf("the garage holds one part in a folder under it, and the deep listing matched %d", got)
	}
}

// The database is bounded by the disk or by the Postgres server, so the usage
// names no limit, and the page draws no meter.
func TestTheUsageIsTheSizeOfTheDatabase(t *testing.T) {
	t.Parallel()

	held, files := fresh(t)
	part := aPart(t, held, "Bracket")
	aVersion(t, held, files, part.ID, "bracket.stl")

	if usage := held.Usage(context.Background()); usage.Bytes <= 0 || usage.Limit != 0 {
		t.Errorf("the usage reads %+v", usage)
	}
}

// A catalogue with no database yet refuses a request, and the request can be
// made again once the module opened one.
func TestACatalogueWithNoDatabaseRefusesARequest(t *testing.T) {
	t.Parallel()

	held := catalog.New(nil, &hostfake.Files{}, place, at)
	if err := held.Ensure(context.Background()); !errors.Is(err, catalog.ErrStore) {
		t.Errorf("a catalogue with no database answered %v", err)
	}
}

func TestAFieldOverItsLimitIsRefused(t *testing.T) {
	t.Parallel()

	held, _ := fresh(t)
	ctx := context.Background()

	for field, draft := range map[string]catalog.PartDraft{
		"an empty name":         {Name: new("  ")},
		"a name with a newline": {Name: new("one\ntwo")},
		"a long description":    {Name: new("x"), Description: new(strings.Repeat("a", catalog.MaxDescriptionBytes+1))},
		"a tag with a space":    {Name: new("x"), Tags: &[]string{"two words"}},
		"seventeen tags": {Name: new("x"), Tags: &[]string{
			"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q",
		}},
		"a folder that is not there": {Name: new("x"), Folder: new(42)},
	} {
		if _, err := held.AddPart(ctx, draft, "user:alice"); err == nil {
			t.Errorf("%s was accepted", field)
		}
	}
}

func TestAPrintFileWithoutAPictureIsRecordedAsSuch(t *testing.T) {
	t.Parallel()

	for _, engine := range dbtest.Engines(t) {
		t.Run(string(engine.Name), func(t *testing.T) {
			t.Parallel()

			held, files := over(t, engine.Grant(t), nil, &hostfake.Files{})
			part := aPart(t, held, "Hinge")
			bare := catalog.PartDir(place.Root, part.ID) + "/up/bare.bgcode"
			files.PutBare(place.Storage, bare, 4, "digest")

			version, err := held.AddVersion(context.Background(), part.ID,
				catalog.VersionDraft{Files: []catalog.FileDraft{{Path: bare}}}, "user:alice")
			if err != nil {
				t.Fatalf("add a version: %v", err)
			}

			if got := version.Files[0].Thumbnail; got == nil || *got {
				t.Errorf("the file has no picture, and Thumbnail reads %v", got)
			}

			detail, err := held.Detail(part.ID)
			if err != nil {
				t.Fatalf("read the part: %v", err)
			}

			if detail.Picture != nil {
				t.Errorf("a part whose only file has no picture drew %+v", detail.Picture)
			}
		})
	}
}

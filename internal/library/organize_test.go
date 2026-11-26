package library

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestOrganizeSuggestsNamesForMisnamedFiles(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	lib := New(store.Client, nil, tasks.NewService(store.Client, tasks.NewRegistry()), nil, Options{})

	// A movie whose file name carries the code: never suggested.
	good := store.Client.Movie.Create().SetCode("ABP-001").SaveX(ctx)
	store.Client.File.Create().SetFileID("f-good").SetName("ABP-001.mp4").SetSize(1).
		SetPath(filepath.Join(root, "ABP-001.mp4")).
		SetAccountID(domain.LocalAccountID).SetRootID(domain.LocalAccountID).
		SetMovieID(good.ID).SaveX(ctx)

	// A movie whose file name misses the code but its folder carries it:
	// the folder fix covers it, no suggestion.
	folder := filepath.Join(root, "ABP-002 (2023)")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	inFolder := store.Client.Movie.Create().SetCode("ABP-002").SaveX(ctx)
	store.Client.File.Create().SetFileID("f-folder").SetName("movie.part2.mp4").SetSize(1).
		SetPath(filepath.Join(folder, "movie.part2.mp4")).
		SetAccountID(domain.LocalAccountID).SetRootID(domain.LocalAccountID).
		SetMovieID(inFolder.ID).SaveX(ctx)

	// A movie with neither file nor folder carrying the code: suggested.
	bad := store.Client.Movie.Create().SetCode("MIDV-003").SaveX(ctx)
	store.Client.File.Create().SetFileID("f-bad").SetName("random_release.mp4").SetSize(1).
		SetPath(filepath.Join(root, "random_release.mp4")).
		SetAccountID(domain.LocalAccountID).SetRootID(domain.LocalAccountID).
		SetMovieID(bad.ID).SaveX(ctx)

	// A movie whose .m4v file misses the code: suggested WITH the extension
	// kept (scanner-recognized via domain.IsVideo).
	store.Client.Movie.Create().SetCode("MIDV-004").SaveX(ctx)
	m4v := store.Client.Movie.Query().Where(movie.CodeEQ("MIDV-004")).OnlyX(ctx)
	store.Client.File.Create().SetFileID("f-m4v").SetName("weird_clip.m4v").SetSize(1).
		SetPath(filepath.Join(root, "weird_clip.m4v")).
		SetAccountID(domain.LocalAccountID).SetRootID(domain.LocalAccountID).
		SetMovieID(m4v.ID).SaveX(ctx)

	// A movie whose file has a non-media extension: skipped entirely —
	// renaming away the extension would corrupt the name.
	store.Client.Movie.Create().SetCode("MIDV-009").SaveX(ctx)
	dat := store.Client.Movie.Query().Where(movie.CodeEQ("MIDV-009")).OnlyX(ctx)
	store.Client.File.Create().SetFileID("f-dat").SetName("weird_blob.dat").SetSize(1).
		SetPath(filepath.Join(root, "weird_blob.dat")).
		SetAccountID(domain.LocalAccountID).SetRootID(domain.LocalAccountID).
		SetMovieID(dat.ID).SaveX(ctx)

	report, err := lib.Organize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.Undoable {
		t.Fatal("report.Undoable = true with an empty journal")
	}
	if len(report.Issues) != 2 {
		t.Fatalf("issues = %+v, want exactly two misnamed_file entries", report.Issues)
	}
	issue := report.Issues[0]
	if issue.Kind != "misnamed_file" || issue.Code != "MIDV-003" {
		t.Fatalf("issue = %+v, want misnamed_file for MIDV-003", issue)
	}
	if issue.SuggestedName != "MIDV-003.mp4" {
		t.Fatalf("suggested name = %q, want MIDV-003.mp4", issue.SuggestedName)
	}
	if issue.FileID == 0 {
		t.Fatal("issue carries no file id; apply would be impossible")
	}
	m4vIssue := report.Issues[1]
	if m4vIssue.SuggestedName != "MIDV-004.m4v" {
		t.Fatalf("m4v suggested name = %q, want MIDV-004.m4v", m4vIssue.SuggestedName)
	}
}

func TestOrganizeApplyRenamesAndUndoRestores(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	lib := New(store.Client, nil, tasks.NewService(store.Client, tasks.NewRegistry()), nil, Options{})

	movie := store.Client.Movie.Create().SetCode("MIDV-003").SaveX(ctx)
	original := filepath.Join(root, "random_release.mp4")
	if err := os.WriteFile(original, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	record := store.Client.File.Create().SetFileID("f-bad").SetName("random_release.mp4").SetSize(1).
		SetPath(original).
		SetAccountID(domain.LocalAccountID).SetRootID(domain.LocalAccountID).
		SetMovieID(movie.ID).SaveX(ctx)

	report, err := lib.ApplyOrganize(ctx, []int{record.ID})
	if err != nil {
		t.Fatal(err)
	}
	renamed := filepath.Join(root, "MIDV-003.mp4")
	if len(report.Renames) != 1 || report.Renames[0].Error != "" || !report.Undoable {
		t.Fatalf("apply report = %+v, want one clean rename and undoable", report.Renames)
	}
	if report.Renames[0].To != renamed {
		t.Fatalf("rename target = %q, want %q", report.Renames[0].To, renamed)
	}
	if _, err := os.Stat(renamed); err != nil {
		t.Fatalf("renamed file missing on disk: %v", err)
	}
	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Fatalf("original file still on disk (err = %v)", err)
	}
	fresh, err := store.Client.File.Get(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Path != renamed || fresh.Name != "MIDV-003.mp4" {
		t.Fatalf("index not updated: path=%q name=%q", fresh.Path, fresh.Name)
	}

	undo, err := lib.UndoOrganize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(undo.Renames) != 1 || undo.Renames[0].Error != "" || undo.Undoable {
		t.Fatalf("undo report = %+v, want one clean rename and nothing left to undo", undo.Renames)
	}
	if _, err := os.Stat(original); err != nil {
		t.Fatalf("restored file missing on disk: %v", err)
	}
	if _, err := os.Stat(renamed); !os.IsNotExist(err) {
		t.Fatalf("renamed file still on disk (err = %v)", err)
	}
	fresh, err = store.Client.File.Get(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Path != original || fresh.Name != "random_release.mp4" {
		t.Fatalf("index not restored: path=%q name=%q", fresh.Path, fresh.Name)
	}

	if _, err := lib.UndoOrganize(ctx); err == nil {
		t.Fatal("second undo succeeded; want an invalid-state error")
	}
}

func TestOrganizeUndoRetriesFailedEntries(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	lib := New(store.Client, nil, tasks.NewService(store.Client, tasks.NewRegistry()), nil, Options{})

	movie := store.Client.Movie.Create().SetCode("MIDV-005").SaveX(ctx)
	original := filepath.Join(root, "random_release.mp4")
	if err := os.WriteFile(original, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	record := store.Client.File.Create().SetFileID("f-gone").SetName("random_release.mp4").SetSize(1).
		SetPath(original).
		SetAccountID(domain.LocalAccountID).SetRootID(domain.LocalAccountID).
		SetMovieID(movie.ID).SaveX(ctx)

	if _, err := lib.ApplyOrganize(ctx, []int{record.ID}); err != nil {
		t.Fatal(err)
	}
	renamed := filepath.Join(root, "MIDV-005.mp4")
	// Simulate the file being moved away after the apply: the first undo must
	// fail, keep the journal entry, and stay retryable.
	if err := os.Remove(renamed); err != nil {
		t.Fatal(err)
	}
	first, err := lib.UndoOrganize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Renames) != 1 || first.Renames[0].Error == "" || !first.Undoable {
		t.Fatalf("first undo = %+v, want one failed rename that stays undoable", first.Renames)
	}
	if err := os.WriteFile(renamed, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	second, err := lib.UndoOrganize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Renames) != 1 || second.Renames[0].Error != "" || second.Undoable {
		t.Fatalf("second undo = %+v, want a clean rename and an empty journal", second.Renames)
	}
	if _, err := os.Stat(original); err != nil {
		t.Fatalf("restored file missing on disk: %v", err)
	}
}

package scrape

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/nfo"
)

func TestSaveMovieMetadataKeepsRenamedActorsAsAliases(t *testing.T) {
	tempDir := t.TempDir()
	store, err := database.Open(t.Context(), filepath.Join(tempDir, "data"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	record, err := store.Client.Movie.Create().SetCode("ABP-001").SetTitle("初版").Save(t.Context())
	if err != nil {
		t.Fatalf("create movie: %v", err)
	}

	tx, err := store.Client.Tx(t.Context())
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := SaveMovieMetadata(t.Context(), tx, record.ID, nfo.Movie{
		Code: "ABP-001", Title: "第一次刮削",
		Actors: []nfo.Actor{{Provider: "javdb", ID: "a1", Name: "旧名", Gender: "female"}},
	}); err != nil {
		t.Fatalf("save first scrape: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit first scrape: %v", err)
	}

	tx, err = store.Client.Tx(t.Context())
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := SaveMovieMetadata(t.Context(), tx, record.ID, nfo.Movie{
		Code: "ABP-001", Title: "第二次刮削",
		Actors: []nfo.Actor{{Provider: "javdb", ID: "a1", Name: "新名", Gender: "female"}},
	}); err != nil {
		t.Fatalf("save renamed scrape: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit renamed scrape: %v", err)
	}

	person, err := store.Client.Actor.Query().Only(t.Context())
	if err != nil {
		t.Fatalf("load actor: %v", err)
	}
	if person.Name != "新名" {
		t.Fatalf("actor name = %q, want 新名", person.Name)
	}
	if !slices.Equal(person.Aliases, []string{"旧名"}) {
		t.Fatalf("actor aliases = %v, want [旧名]", person.Aliases)
	}

	// A repeated scrape with the same name plus manually authored aliases must
	// keep the accumulated spellings.
	tx, err = store.Client.Tx(t.Context())
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := SaveMovieMetadata(t.Context(), tx, record.ID, nfo.Movie{
		Code: "ABP-001", Title: "第三次刮削",
		Actors: []nfo.Actor{{Provider: "javdb", ID: "a1", Name: "新名", Gender: "female", Aliases: []string{"别名A"}}},
	}); err != nil {
		t.Fatalf("save third scrape: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit third scrape: %v", err)
	}
	person, err = store.Client.Actor.Query().Only(t.Context())
	if err != nil {
		t.Fatalf("reload actor: %v", err)
	}
	if person.Name != "新名" {
		t.Fatalf("actor name after manual aliases = %q, want 新名", person.Name)
	}
	if !slices.Equal(person.Aliases, []string{"旧名", "别名A"}) {
		t.Fatalf("actor aliases = %v, want [旧名 别名A]", person.Aliases)
	}
	linked, err := person.QueryMovies().IDs(t.Context())
	if err != nil {
		t.Fatalf("query linked movies: %v", err)
	}
	if len(linked) != 1 || linked[0] != record.ID {
		t.Fatalf("movie links = %v, want [%d]", linked, record.ID)
	}
}

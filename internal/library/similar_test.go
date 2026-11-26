package library

import (
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestSimilarRanksSharedLinksAndSkipsOutliers(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	created := store.Client.Movie.CreateBulk(
		store.Client.Movie.Create().SetCode("SSIS-001").SetTitle("Base").SetReleaseDate(time.Now()).
			SetSeriesID("javdb:s-1").SetSeriesName("Series").
			SetMakerID("javdb:m-1").SetMakerName("Maker"),
		store.Client.Movie.Create().SetCode("SSIS-002").SetTitle("SameSeries").SetReleaseDate(time.Now()).
			SetSeriesID("javdb:s-1").SetSeriesName("Series"),
		store.Client.Movie.Create().SetCode("MIDV-001").SetTitle("SameMaker").SetReleaseDate(time.Now()).
			SetMakerID("javdb:m-1").SetMakerName("Maker"),
		store.Client.Movie.Create().SetCode("FC2-PPV-1").SetTitle("Unrelated").SetReleaseDate(time.Now()),
	).SaveX(ctx)
	baseMovie, twinMovie, looseMovie, strayMovie := created[0], created[1], created[2], created[3]

	actorOnly := store.Client.Movie.Create().SetCode("SSIS-003").SetTitle("ActorOnly").SetReleaseDate(time.Now()).SaveX(ctx)
	tagOnly := store.Client.Movie.Create().SetCode("MIDV-002").SetTitle("TagOnly").SetReleaseDate(time.Now()).SaveX(ctx)

	actor := store.Client.Actor.Create().SetProvider("javdb").SetSourceID("a-1").SetName("Actor").
		AddMovies(baseMovie, twinMovie, actorOnly).SaveX(ctx)
	label := store.Client.Tag.Create().SetProvider("javdb").SetSourceID("t-1").SetName("Tag").
		AddMovies(baseMovie, looseMovie, tagOnly).SaveX(ctx)
	_ = actor
	_ = label

	for _, record := range append(created, actorOnly, tagOnly) {
		store.Client.File.Create().SetFileID("f-" + record.Code).SetName(record.Code + ".mp4").
			SetSize(1).SetPath("/media/" + record.Code + ".mp4").
			SetAccountID(domain.LocalAccountID).SetRootID(domain.LocalAccountID).
			SetMovie(record).ExecX(ctx)
	}

	service := New(store.Client, nil, tasks.NewService(store.Client, tasks.NewRegistry()), nil, Options{})
	movies, err := service.Similar(ctx, baseMovie.ID)
	if err != nil {
		t.Fatal(err)
	}

	got := make([]string, 0, len(movies))
	for _, item := range movies {
		got = append(got, item.Code)
	}
	// twin shares actor+series (score 3); actorOnly shares the actor (2);
	// loose shares tag+maker (2, older id so it sorts after actorOnly on the
	// stable tie-break); tagOnly shares the tag (1). Unrelated is excluded.
	want := []string{"SSIS-002", "SSIS-003", "MIDV-001", "MIDV-002"}
	if len(got) != len(want) {
		t.Fatalf("similar movies: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("similar order: got %v want %v", got, want)
		}
	}

	if _, err := service.Similar(ctx, strayMovie.ID); err != nil {
		t.Fatalf("similar for a movie without links: %v", err)
	}
	// A movie with no actors, tags, series or maker yields an empty list.
	empty := store.Client.Movie.Create().SetCode("EMPTY-001").SetTitle("NoLinks").SetReleaseDate(time.Now()).SaveX(ctx)
	store.Client.File.Create().SetFileID("f-empty").SetName("empty.mp4").SetSize(1).SetPath("/media/empty.mp4").
		SetAccountID(domain.LocalAccountID).SetRootID(domain.LocalAccountID).SetMovie(empty).ExecX(ctx)
	list, err := service.Similar(ctx, empty.ID)
	if err != nil || len(list) != 0 {
		t.Fatalf("no-link movie: len=%d err=%v", len(list), err)
	}

	if _, err := service.Similar(ctx, 9999); err == nil {
		t.Fatal("expected not-found error for a missing movie")
	}
}

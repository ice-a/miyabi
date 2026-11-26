package library

import (
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestFavoriteToggleAndFilter(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	created := store.Client.Movie.CreateBulk(
		store.Client.Movie.Create().SetCode("ABP-001").SetTitle("Fav").SetReleaseDate(time.Now()),
		store.Client.Movie.Create().SetCode("IPX-123").SetTitle("Plain").SetReleaseDate(time.Now()),
	).SaveX(ctx)
	for _, record := range created {
		store.Client.File.Create().SetFileID("f-" + record.Code).SetName(record.Code + ".mp4").
			SetSize(1).SetPath("/media/" + record.Code + ".mp4").
			SetAccountID(domain.LocalAccountID).SetRootID(domain.LocalAccountID).
			SetMovie(record).ExecX(ctx)
	}

	service := New(store.Client, nil, tasks.NewService(store.Client, tasks.NewRegistry()), nil, Options{})
	fav := created[0].ID

	if err := service.SetFavorite(ctx, fav, true); err != nil {
		t.Fatal(err)
	}
	page, err := service.Movies(ctx, 1, 20, true)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Movies) != 1 || page.Movies[0].ID != fav || !page.Movies[0].Favorite {
		t.Fatalf("favorite filter: total=%d movies=%+v", page.Total, page.Movies)
	}

	all, err := service.Movies(ctx, 1, 20, false)
	if err != nil || all.Total != 2 {
		t.Fatalf("unfiltered list: total=%d err=%v", all.Total, err)
	}
	for _, item := range all.Movies {
		if item.ID == fav && !item.Favorite {
			t.Fatal("favorite flag lost in unfiltered projection")
		}
	}

	if err := service.SetFavorite(ctx, fav, false); err != nil {
		t.Fatal(err)
	}
	empty, err := service.Movies(ctx, 1, 20, true)
	if err != nil || empty.Total != 0 || len(empty.Movies) != 0 {
		t.Fatalf("favorites after untoggle: total=%d len=%d err=%v", empty.Total, len(empty.Movies), err)
	}
	if err := service.SetFavorite(ctx, fav, true); err != nil {
		t.Fatal(err)
	}
	if err := service.SetFavorite(ctx, 9999, true); err == nil {
		t.Fatal("expected not-found error for a missing movie")
	}
	detail, err := service.Movie(ctx, fav)
	if err != nil || !detail.Favorite {
		t.Fatalf("detail favorite flag: %+v err=%v", detail.Favorite, err)
	}
}

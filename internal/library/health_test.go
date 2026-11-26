package library

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestHealthReportsIndexProblems(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	base := t.TempDir()
	live := filepath.Join(base, "ABP-001.mp4")
	gone := filepath.Join(base, "MISSING", "IPX-123.mp4")
	if err := os.WriteFile(live, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := store.Client.Movie.CreateBulk(
		store.Client.Movie.Create().SetCode("ABP-001").SetTitle("Live").SetReleaseDate(time.Now()).SetScrapeStatus(movie.ScrapeStatusDone),
		store.Client.Movie.Create().SetCode("IPX-123").SetTitle("Missing").SetReleaseDate(time.Now()).SetScrapeStatus(movie.ScrapeStatusDone),
		store.Client.Movie.Create().SetCode("IPX-124").SetTitle("Ghost").SetReleaseDate(time.Now()).SetScrapeStatus(movie.ScrapeStatusDone),
		store.Client.Movie.Create().SetCode("IPX-125").SetTitle("Broken").SetReleaseDate(time.Now()).SetScrapeStatus(movie.ScrapeStatusFailed),
	).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	liveMovie, err := store.Client.Movie.Query().Where(movie.Code("ABP-001")).Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	missingMovie, err := store.Client.Movie.Query().Where(movie.Code("IPX-123")).Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Client.File.Create().SetFileID("f1").SetName("ABP-001.mp4").SetSize(1).SetPath(live).SetMovie(liveMovie).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Client.File.Create().SetFileID("f2").SetName("IPX-123.mp4").SetSize(1).SetPath(gone).SetMovie(missingMovie).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	service := New(store.Client, nil, tasks.NewService(store.Client, tasks.NewRegistry()), nil, Options{})
	report, err := service.Health(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.Movies != 4 || report.Files != 2 {
		t.Fatalf("report totals: movies=%d files=%d", report.Movies, report.Files)
	}
	kinds := map[string]int{}
	for _, issue := range report.Issues {
		kinds[issue.Kind]++
	}
	if kinds["missing_file"] != 1 || kinds["missing_file_summary"] != 1 {
		t.Fatalf("missing-file issues missing: %v", kinds)
	}
	if kinds["orphan_movie"] != 2 {
		t.Fatalf("expected two orphan movies, got %v", kinds)
	}
	if kinds["unscraped"] != 1 {
		t.Fatalf("expected one unscraped movie, got %v", kinds)
	}
	for _, issue := range report.Issues {
		switch issue.Kind {
		case "missing_file":
			if issue.Path != gone {
				t.Fatalf("missing file path = %q", issue.Path)
			}
		case "orphan_movie":
			if issue.Code != "IPX-124" && issue.Code != "IPX-125" {
				t.Fatalf("orphan movie = %#v", issue)
			}
		case "unscraped":
			if issue.Code != "IPX-125" {
				t.Fatalf("unscraped movie = %#v", issue)
			}
		}
	}
}

package library

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
)

// issueLimit keeps one report response bounded; the totals still show up in
// the summary issues, and re-running the report is cheap.
const issueLimit = 100

// HealthIssue is one detected problem, keyed by kind:
// "missing_file" (indexed file gone from disk), "unscraped" (metadata not
// done) and "orphan_movie" (no files attached).
// *_summary issues carry the total count when a category was truncated.
type HealthIssue struct {
	Kind  string `json:"kind"`
	Code  string `json:"code,omitempty"`
	Title string `json:"title,omitempty"`
	Path  string `json:"path,omitempty"`
	Count int    `json:"count,omitempty"`
}

// HealthReport is the library check-up served by GET /api/library/health.
type HealthReport struct {
	GeneratedAt time.Time     `json:"generated_at"`
	Movies      int           `json:"movies"`
	Files       int           `json:"files"`
	Issues      []HealthIssue `json:"issues"`
}

// Health scans the persisted index and the mounted media directory, then
// reports the problems it can detect without touching upstream sources.
func (s *Service) Health(ctx context.Context) (HealthReport, error) {
	report := HealthReport{GeneratedAt: time.Now(), Issues: []HealthIssue{}}

	movieCount, err := s.database.Movie.Query().Count(ctx)
	if err != nil {
		return report, fmt.Errorf("count movies: %w", err)
	}
	report.Movies = movieCount

	files, err := s.database.File.Query().
		Select(file.FieldPath).
		All(ctx)
	if err != nil {
		return report, fmt.Errorf("list files: %w", err)
	}
	report.Files = len(files)

	missing := 0
	for _, record := range files {
		if _, err := os.Stat(record.Path); err != nil {
			missing++
			if len(report.Issues) < issueLimit {
				report.Issues = append(report.Issues, HealthIssue{Kind: "missing_file", Path: record.Path})
			}
		}
	}
	if missing > 0 {
		report.Issues = append(report.Issues, HealthIssue{Kind: "missing_file_summary", Count: missing})
	}

	unscraped := movie.ScrapeStatusNEQ(movie.ScrapeStatusDone)
	records, err := s.database.Movie.Query().
		Where(unscraped).
		Select(movie.FieldCode, movie.FieldTitle).
		Limit(issueLimit).
		All(ctx)
	if err != nil {
		return report, fmt.Errorf("list unscraped movies: %w", err)
	}
	unscrapedCount, err := s.database.Movie.Query().Where(unscraped).Count(ctx)
	if err != nil {
		return report, fmt.Errorf("count unscraped movies: %w", err)
	}
	for _, record := range records {
		report.Issues = append(report.Issues, HealthIssue{
			Kind: "unscraped", Code: record.Code, Title: record.Title,
		})
	}
	if unscrapedCount > len(records) {
		report.Issues = append(report.Issues, HealthIssue{Kind: "unscraped_summary", Count: unscrapedCount})
	}

	orphanScope := movie.Not(movie.HasFiles())
	orphans, err := s.database.Movie.Query().
		Where(orphanScope).
		Select(movie.FieldCode, movie.FieldTitle).
		Limit(issueLimit).
		All(ctx)
	if err != nil {
		return report, fmt.Errorf("list orphan movies: %w", err)
	}
	orphanCount, err := s.database.Movie.Query().Where(orphanScope).Count(ctx)
	if err != nil {
		return report, fmt.Errorf("count orphan movies: %w", err)
	}
	for _, record := range orphans {
		report.Issues = append(report.Issues, HealthIssue{
			Kind: "orphan_movie", Code: record.Code, Title: record.Title,
		})
	}
	if orphanCount > len(orphans) {
		report.Issues = append(report.Issues, HealthIssue{Kind: "orphan_movie_summary", Count: orphanCount})
	}

	return report, nil
}

package library

import (
	"context"
	"fmt"
	"sort"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/predicate"
	"github.com/ppxb/miyabi/internal/ent/tag"
)

const (
	// similarLimit caps the rendered recommendation list.
	similarLimit = 12
	// similarCandidates bounds the SQL pre-filter before in-memory scoring.
	similarCandidates = 80
)

// similarCandidate pairs a movie with its shared-link score so the ranking
// sort can never drift out of sync with the scores.
type similarCandidate struct {
	movie Movie
	score int
}

// Similar lists library movies that share actors, tags, a series or a maker
// with the given movie, ranked by how many of those links they have in
// common. Shared actors weigh heavier than shared tags: actors identify a
// title far more precisely than broad genre labels.
func (s *Service) Similar(ctx context.Context, id int) ([]Movie, error) {
	record, err := s.detailRecord(ctx, id)
	if err != nil {
		return nil, err
	}
	labels, err := s.database.Tag.Query().Where(tag.HasMoviesWith(movie.IDEQ(id))).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load movie tags: %w", err)
	}

	links := make([]predicate.Movie, 0, len(record.Edges.Actors)+len(labels)+2)
	for _, person := range record.Edges.Actors {
		links = append(links, movie.HasActorsWith(actor.IDEQ(person.ID)))
	}
	for _, label := range labels {
		links = append(links, movie.HasTagsWith(tag.IDEQ(label.ID)))
	}
	if record.SeriesID != nil {
		links = append(links, movie.SeriesIDEQ(*record.SeriesID))
	}
	if record.MakerID != nil {
		links = append(links, movie.MakerIDEQ(*record.MakerID))
	}
	if len(links) == 0 {
		return []Movie{}, nil
	}

	// The SQL pre-filter keeps the candidate set small; the shared-link count
	// is computed in memory because ent exposes no aggregated edge count.
	candidates, err := s.database.Movie.Query().
		Where(movie.IDNEQ(id), movie.HasFilesWith(s.scope(ctx)), movie.Or(links...)).
		Select(movie.FieldID, movie.FieldCode, movie.FieldTitle, movie.FieldJavdbID, movie.FieldCover, movie.FieldPoster,
			movie.FieldFanarts, movie.FieldReleaseDate, movie.FieldDuration, movie.FieldRating,
			movie.FieldMetadata, movie.FieldFavorite,
			movie.FieldDirectorID, movie.FieldDirectorName, movie.FieldMakerID, movie.FieldMakerName,
			movie.FieldSeriesID, movie.FieldSeriesName,
			movie.FieldScrapeStatus).
		Order(ent.Desc(movie.FieldCreatedAt), ent.Desc(movie.FieldID)).
		Limit(similarCandidates).
		WithActors(func(query *ent.ActorQuery) {
			query.Select(actor.FieldID).Order(ent.Asc(actor.FieldID))
		}).
		WithTags(func(query *ent.TagQuery) {
			query.Select(tag.FieldID).Order(ent.Asc(tag.FieldID))
		}).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list similar movies: %w", err)
	}

	actorIDs := make(map[int]bool, len(record.Edges.Actors))
	for _, person := range record.Edges.Actors {
		actorIDs[person.ID] = true
	}
	tagIDs := make(map[int]bool, len(labels))
	for _, label := range labels {
		tagIDs[label.ID] = true
	}

	scored := make([]similarCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		score := 0
		if record.SeriesID != nil && candidate.SeriesID != nil && *record.SeriesID == *candidate.SeriesID {
			score++
		}
		if record.MakerID != nil && candidate.MakerID != nil && *record.MakerID == *candidate.MakerID {
			score++
		}
		for _, person := range candidate.Edges.Actors {
			if actorIDs[person.ID] {
				score += 2
			}
		}
		for _, label := range candidate.Edges.Tags {
			if tagIDs[label.ID] {
				score++
			}
		}
		if score == 0 {
			continue
		}
		scored = append(scored, similarCandidate{movie: movieItem(candidate), score: score})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].movie.ID > scored[j].movie.ID
	})
	if len(scored) > similarLimit {
		scored = scored[:similarLimit]
	}
	movies := make([]Movie, 0, len(scored))
	for _, item := range scored {
		movies = append(movies, item.movie)
	}
	if err := s.markCloudSources(ctx, movies, cloudFileScope(s.cloudSources(ctx))); err != nil {
		return nil, err
	}
	return movies, nil
}

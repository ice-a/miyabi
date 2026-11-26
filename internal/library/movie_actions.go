package library

import (
	"context"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
)

func (s *Service) RescrapeMovie(ctx context.Context, id int, code string) (domain.TaskInfo, error) {
	parent, err := s.scanner.RescrapeMovie(ctx, id, code)
	if err != nil {
		return domain.TaskInfo{}, err
	}
	infos, err := s.Workflows(ctx, []*ent.Task{parent})
	if err != nil {
		return domain.TaskInfo{}, err
	}
	return infos[0], nil
}

// SetFavorite toggles the user-curated flag on one library movie.
func (s *Service) SetFavorite(ctx context.Context, id int, favorite bool) error {
	affected, err := s.database.Movie.Update().Where(movie.IDEQ(id), movie.HasFilesWith(s.scope(ctx))).
		SetFavorite(favorite).
		Save(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.E(domain.KindNotFound, "影片不在当前媒体库中", nil)
	}
	s.tasks.NotifyLibraryChanged()
	return nil
}

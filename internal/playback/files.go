package playback

import (
	"context"
	"fmt"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/predicate"
	"github.com/ppxb/miyabi/internal/storage"
)

// Files lists the playable videos of one library movie.
func (service *Service) Files(ctx context.Context, movieID int) (PlayFiles, error) {
	source, err := service.movieSource(ctx, movieID)
	if err != nil {
		return PlayFiles{}, err
	}
	scope := libraryFiles(source)
	record, err := service.database.Movie.Query().
		Where(movie.IDEQ(movieID), movie.HasFilesWith(scope)).
		WithFiles(func(query *ent.FileQuery) {
			query.Where(scope).Order(ent.Desc(file.FieldSize), ent.Asc(file.FieldPath), ent.Asc(file.FieldID))
		}).Only(ctx)
	if err != nil {
		return PlayFiles{}, fmt.Errorf("read playable movie: %w", err)
	}
	result := PlayFiles{
		Code: record.Code, Title: record.Title, Source: source,
		Files: make([]domain.LibraryFile, 0, len(record.Edges.Files)),
	}
	for _, entry := range record.Edges.Files {
		result.Files = append(result.Files, domain.LibraryFile{ID: entry.FileID, Name: entry.Name, Path: entry.Path, Size: entry.Size})
	}
	return result, nil
}

// Start creates a playback session for one indexed video file.
func (service *Service) Start(ctx context.Context, fileID, userAgent string) (Playback, error) {
	opened, err := service.openFile(ctx, fileID)
	if err != nil {
		return Playback{}, err
	}
	result, err := service.createSession(opened.source, mediaSpecs(opened))
	if err != nil {
		return Playback{}, err
	}
	result.FileID = fileID
	return result, nil
}

// StartHLS is kept for clients that still ask for a transcoded session: the
// local library serves the file itself, so both entry points return the same
// progressive source.
func (service *Service) StartHLS(ctx context.Context, fileID, userAgent string) (Playback, error) {
	return service.Start(ctx, fileID, userAgent)
}

type openedFile struct {
	source domain.LibrarySource
	id     string
	name   string
}

func (service *Service) openFile(ctx context.Context, fileID string) (openedFile, error) {
	record, err := service.database.File.Query().Where(file.FileIDEQ(fileID)).
		Select(file.FieldAccountID, file.FieldRootID, file.FieldFileID, file.FieldName).Only(ctx)
	if err != nil {
		return openedFile{}, fmt.Errorf("read indexed video: %w", err)
	}
	return service.openFileRecord(ctx, record)
}

func (service *Service) openFileRecord(ctx context.Context, record *ent.File) (openedFile, error) {
	source, err := service.sourceFor(ctx, record.AccountID, record.RootID)
	if err != nil {
		return openedFile{}, err
	}
	if _, err := service.storage.OpenSource(ctx, source); err != nil {
		return openedFile{}, err
	}
	if !domain.IsVideo(record.Name) {
		return openedFile{}, domain.E(domain.KindNotFound, "视频已不在当前媒体目录中，请重新扫描", nil)
	}
	return openedFile{source: source, id: record.FileID, name: record.Name}, nil
}

// StartMovie starts playback for a movie's primary video in one call.
func (service *Service) StartMovie(ctx context.Context, movieID int, userAgent string) (Playback, error) {
	records, err := service.database.File.Query().
		Where(file.MovieIDEQ(movieID)).
		Select(file.FieldFileID, file.FieldSize, file.FieldAccountID, file.FieldRootID, file.FieldName, file.FieldPath).
		Order(ent.Desc(file.FieldSize), ent.Asc(file.FieldPath), ent.Asc(file.FieldID)).All(ctx)
	if err != nil {
		return Playback{}, fmt.Errorf("read indexed videos: %w", err)
	}
	if len(records) == 0 {
		return Playback{}, domain.E(domain.KindNotFound, "当前媒体目录中未找到该影片的媒体文件", nil)
	}
	resume, err := service.Resume(ctx, movieID)
	if err != nil {
		return Playback{}, err
	}
	target := records[0]
	if resume != nil {
		for _, item := range records {
			if item.FileID == resume.FileID {
				target = item
				break
			}
		}
		if target.FileID != resume.FileID {
			resume = nil
		}
	}
	opened, err := service.openFileRecord(ctx, target)
	if err != nil {
		return Playback{}, err
	}
	result, err := service.createSession(opened.source, mediaSpecs(opened))
	if err != nil {
		return Playback{}, err
	}
	result.FileID = target.FileID
	result.Resume = resume
	return result, nil
}

// mediaSpecs addresses the indexed file itself: the player streams it through
// this server, which supports byte ranges for seeking.
func mediaSpecs(opened openedFile) []mediaSpec {
	return []mediaSpec{{
		fileID:    opened.id,
		mediaType: storage.MediaType(opened.name),
		label:     "原画",
	}}
}

func (service *Service) movieSource(ctx context.Context, movieID int) (domain.LibrarySource, error) {
	record, err := service.database.File.Query().
		Where(file.MovieIDEQ(movieID)).
		Select(file.FieldAccountID, file.FieldRootID).First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return domain.LibrarySource{}, domain.E(domain.KindNotFound, "当前媒体目录中未找到该影片的媒体文件", nil)
		}
		return domain.LibrarySource{}, err
	}
	return service.sourceFor(ctx, record.AccountID, record.RootID)
}

func (service *Service) sourceFor(ctx context.Context, accountID, rootID string) (domain.LibrarySource, error) {
	if primary := service.storage.Source(); primary != nil && primary.AccountID == accountID && primary.Directory.ID == rootID {
		return *primary, nil
	}
	return domain.LibrarySource{}, domain.E(domain.KindNotFound, "当前媒体目录中未找到该影片的媒体文件", nil)
}

// libraryFiles scopes a query to the files of one mounted source.
func libraryFiles(source domain.LibrarySource) predicate.File {
	return file.And(file.AccountIDEQ(source.AccountID), file.RootIDEQ(source.Directory.ID))
}

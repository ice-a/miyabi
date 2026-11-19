package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
)

// Session reads one library source: a directory tree of indexed media files
// plus the transaction boundary its derived updates are written through.
//
// Scanning, scraping and playback only depend on this interface, so a source
// can be backed by a local disk today and by another directory service later.
type Session interface {
	Source() domain.LibrarySource
	// List returns one page of directory entries starting at offset.
	List(ctx context.Context, dirID string, offset int) (FilePage, error)
	// Info returns the current entry for a single file ID.
	Info(ctx context.Context, fileID string) (File, error)
	// Read returns up to limit bytes of a small metadata file identified by id.
	Read(ctx context.Context, id string, limit int64) ([]byte, error)
	// OpenStream opens a playable body for the file, honouring Range headers.
	OpenStream(ctx context.Context, fileID string, headers http.Header) (*http.Response, error)
	// WithSource runs fn while the source is still the one the session reads.
	WithSource(ctx context.Context, fn func() error) error
	// Commit runs fn in a database transaction tied to this source.
	Commit(ctx context.Context, fn func(tx *ent.Tx) error) error
}

// localSession serves a mounted local directory tree.
type localSession struct {
	service *Service
	source  domain.LibrarySource
}

// Source reports the mounted local directory.
func (session *localSession) Source() domain.LibrarySource { return session.source }

// List enumerates one local directory and reports the path segments leading to
// it, so callers can verify a walk stays inside the mount.
func (session *localSession) List(ctx context.Context, dirID string, _ int) (FilePage, error) {
	if !session.service.within(dirID) {
		return FilePage{}, domain.E(domain.KindConflict, "该文件夹已移出媒体目录，请重新扫描", nil)
	}
	entries, err := os.ReadDir(dirID)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return FilePage{}, domain.E(domain.KindNotFound, "目录不存在或已被移除，请重新扫描", nil)
		}
		return FilePage{}, err
	}
	files := make([]File, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(dirID, entry.Name())
		if entry.IsDir() {
			files = append(files, File{ID: path, ParentID: dirID, Name: entry.Name(), IsDirectory: true, Path: path})
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return FilePage{}, fmt.Errorf("stat %s: %w", path, err)
		}
		files = append(files, File{
			ID: path, ParentID: dirID, Name: entry.Name(), Size: info.Size(),
			Path: path, UploadTime: info.ModTime().Unix(),
		})
	}
	return FilePage{Files: files, Path: session.segments(dirID), Total: len(files), HasMore: false}, nil
}

// Info reports a single local entry.
func (session *localSession) Info(ctx context.Context, fileID string) (File, error) {
	if !session.service.within(fileID) {
		return File{}, domain.E(domain.KindNotFound, "下载资源已移出媒体目录", fs.ErrNotExist)
	}
	info, err := os.Stat(fileID)
	if err != nil {
		return File{}, domain.E(domain.KindNotFound, "下载资源已移出媒体目录", err)
	}
	return File{
		ID: fileID, ParentID: filepath.Dir(fileID), Name: filepath.Base(fileID),
		Size: info.Size(), IsDirectory: info.IsDir(), Path: fileID, UploadTime: info.ModTime().Unix(),
	}, nil
}

// Read returns bytes of a small metadata file inside the mount.
func (session *localSession) Read(ctx context.Context, id string, limit int64) ([]byte, error) {
	if !session.service.within(id) {
		return nil, domain.E(domain.KindNotFound, "文件已移出媒体目录", fs.ErrNotExist)
	}
	file, err := os.Open(id)
	if err != nil {
		return nil, domain.E(domain.KindNotFound, "文件已移出媒体目录", err)
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, limit))
}

// OpenStream serves local media bytes, letting the caller seek with Range.
func (session *localSession) OpenStream(ctx context.Context, fileID string, headers http.Header) (*http.Response, error) {
	file, info, err := openWithin(session.service.root, fileID)
	if err != nil {
		return nil, err
	}
	response, err := serveFile(file, info, headers)
	if err != nil {
		file.Close()
		return nil, err
	}
	return response, nil
}

// WithSource runs fn after confirming the mount still points at this directory.
func (session *localSession) WithSource(ctx context.Context, fn func() error) error {
	current := session.service.Source()
	if current == nil || current.Directory.ID != session.source.Directory.ID || !session.service.within(session.source.Directory.ID) {
		return ErrSourceChanged
	}
	return fn()
}

// Commit runs fn in a database transaction.
func (session *localSession) Commit(ctx context.Context, fn func(tx *ent.Tx) error) error {
	return session.service.commit(ctx, fn)
}

// segments reports the path from the mount root down to dirID.
func (session *localSession) segments(dirID string) []Directory {
	root := session.source.Directory.ID
	segments := []Directory{{ID: root, Name: session.source.Directory.Name}}
	relative, err := filepath.Rel(root, dirID)
	if err != nil || relative == "." || strings.HasPrefix(relative, "..") {
		return segments
	}
	current := root
	for _, part := range strings.Split(filepath.ToSlash(relative), "/") {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		segments = append(segments, Directory{ID: current, Name: part})
	}
	return segments
}

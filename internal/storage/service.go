package storage

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/setting"
	"github.com/ppxb/miyabi/internal/syncx"
)

// directorySetting stores the mounted local media directory.
const directorySetting = "storage.library_directory"

// remoteTimeout bounds a single remote playlist or media request.
const remoteTimeout = 20 * time.Second

// UserAgent identifies this server when relaying remote media.
const UserAgent = "Miyabi/1.0"

// mountRecord persists which local directory is mounted as the media library.
type mountRecord struct {
	Directory domain.LibraryDirectory `json:"directory"`
}

func (record mountRecord) source() domain.LibrarySource {
	if record.Directory.ID == "" {
		return domain.LibrarySource{}
	}
	return domain.LibrarySource{AccountID: domain.LocalAccountID, Directory: record.Directory}
}

// MountEvent reports a mount change to interested services.
type MountEvent struct {
	Source domain.LibrarySource
}

// MountListener reacts to mount changes, typically by queueing a fresh scan.
type MountListener func(ctx context.Context, event MountEvent) error

// Service owns the local media mount and hands out sessions reading it.
type Service struct {
	database *ent.Client
	client   *http.Client
	write    syncx.ContextLock
	mu       sync.Mutex
	mounted  mountRecord
	root     string
	listener MountListener
}

// New loads the persisted mount. An unreadable or missing setting leaves the
// library unmounted, which is the state a fresh install starts in.
func New(ctx context.Context, client *ent.Client, remote *http.Client) *Service {
	service := &Service{database: client, client: remote}
	if service.client == nil {
		service.client = &http.Client{Timeout: remoteTimeout}
	}
	record, ok, err := database.LoadSetting[mountRecord](ctx, client, directorySetting)
	if err == nil && ok {
		service.mounted = record
		service.root = record.Directory.ID
	}
	return service
}

// Close releases the service. It is safe to call more than once.
func (service *Service) Close() error { return nil }

// SetMountListener binds the callback invoked after a mount change.
func (service *Service) SetMountListener(listener MountListener) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.listener = listener
}

// Source returns the mounted library source, or nil when nothing is mounted.
func (service *Service) Source() *domain.LibrarySource {
	service.mu.Lock()
	defer service.mu.Unlock()
	source := service.mounted.source()
	if source.Directory.ID == "" {
		return nil
	}
	return &source
}

// Open returns a session for the current mount.
func (service *Service) Open(ctx context.Context) (Session, error) {
	source := service.Source()
	if source == nil {
		return nil, domain.E(domain.KindInvalid, "请先在设置页选择媒体目录", nil)
	}
	return service.OpenSource(ctx, *source)
}

// OpenSource returns a session for the given source, refusing any source that
// is no longer mounted so callers cannot apply stale scan results.
func (service *Service) OpenSource(ctx context.Context, source domain.LibrarySource) (Session, error) {
	if source.AccountID != domain.LocalAccountID || source.Directory.ID == "" {
		return nil, ErrSourceChanged
	}
	if current := service.Source(); current == nil || current.Directory.ID != source.Directory.ID {
		return nil, ErrSourceChanged
	}
	if !service.within(source.Directory.ID) {
		return nil, ErrSourceChanged
	}
	return &localSession{service: service, source: source}, nil
}

// Mount makes directory the active media root, persists it and notifies the
// listener before returning, so a queued scan always observes the new mount.
func (service *Service) Mount(ctx context.Context, directory domain.LibraryDirectory) (domain.LibrarySource, error) {
	if directory.ID == "" {
		return domain.LibrarySource{}, domain.E(domain.KindInvalid, "媒体目录不能为空", nil)
	}
	if !service.within(directory.ID) {
		return domain.LibrarySource{}, domain.E(domain.KindNotFound, "目录不存在或已被移除", nil)
	}
	if directory.Name == "" || directory.Path == "" {
		directory.Name = filepath.Base(directory.ID)
		directory.Path = directory.ID
	}
	record := mountRecord{Directory: directory}
	if err := service.write.Lock(ctx); err != nil {
		return domain.LibrarySource{}, err
	}
	defer service.write.Unlock()
	if err := database.SaveSetting(ctx, service.database, directorySetting, record); err != nil {
		return domain.LibrarySource{}, domain.E(domain.KindUnexpected, "无法保存媒体目录设置", err)
	}
	service.mu.Lock()
	previous, previousRoot := service.mounted, service.root
	service.mounted, service.root = record, record.Directory.ID
	listener := service.listener
	service.mu.Unlock()
	source := record.source()
	if listener != nil {
		if err := listener(ctx, MountEvent{Source: source}); err != nil {
			service.mu.Lock()
			service.mounted, service.root = previous, previousRoot
			service.mu.Unlock()
			if rollback := database.SaveSetting(ctx, service.database, directorySetting, previous); rollback != nil {
				return domain.LibrarySource{}, domain.E(domain.KindUnexpected, "挂载媒体目录失败", fmt.Errorf("%w (rollback failed: %v)", err, rollback))
			}
			return domain.LibrarySource{}, domain.E(domain.KindUnexpected, "挂载媒体目录失败", err)
		}
	}
	return source, nil
}

// Clear unmounts the media directory and notifies the listener.
func (service *Service) Clear(ctx context.Context) error {
	if err := service.write.Lock(ctx); err != nil {
		return err
	}
	defer service.write.Unlock()
	if _, err := service.database.Setting.Delete().Where(setting.Key(directorySetting)).Exec(ctx); err != nil {
		return domain.E(domain.KindUnexpected, "无法清除媒体目录设置", err)
	}
	service.mu.Lock()
	service.mounted, service.root = mountRecord{}, ""
	listener := service.listener
	service.mu.Unlock()
	if listener != nil {
		return listener(ctx, MountEvent{})
	}
	return nil
}

// commit runs fn inside a database transaction.
func (service *Service) commit(ctx context.Context, fn func(tx *ent.Tx) error) error {
	return ent.WithTx(ctx, service.database, fn)
}

// rootDirectory reports the mounted media root, or an empty string.
func (service *Service) rootDirectory() string {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.root
}

// within reports whether path is the mounted root or below it.
func (service *Service) within(path string) bool {
	root := service.rootDirectory()
	if root == "" || path == "" {
		return false
	}
	clean := filepath.Clean(path)
	if clean == root {
		return statIsDirectory(clean)
	}
	relative, err := filepath.Rel(root, clean)
	if err != nil || relative == "." || strings.HasPrefix(relative, "..") {
		return false
	}
	return true
}

// statIsDirectory reports whether path names an existing directory.
func statIsDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info != nil && info.IsDir()
}

// Within reports whether a listed entry belongs to the given source.
func Within(file File, source domain.LibrarySource) bool {
	root := source.Directory.ID
	if root == "" || file.ID == root {
		return true
	}
	if within, err := filepath.Rel(filepath.Clean(root), filepath.Clean(file.ID)); err == nil &&
		within != "." && !strings.HasPrefix(within, "..") {
		return true
	}
	return slicesContainsPath(file.Path, root)
}

// slicesContainsPath reports whether any segment of path equals id.
func slicesContainsPath(path, id string) bool {
	if path == "" || id == "" {
		return false
	}
	for _, segment := range strings.Split(filepath.ToSlash(path), "/") {
		if segment == id {
			return true
		}
	}
	return false
}

// containsDirectory reports whether the listed path segments include id.
func containsDirectory(segments []Directory, id string) bool {
	return slices.ContainsFunc(segments, func(directory Directory) bool { return directory.ID == id })
}

// DirectoryEntries lists every file inside one directory of the source.
func DirectoryEntries(ctx context.Context, session Session, dirID string) ([]File, error) {
	var entries []File
	err := WalkPages(ctx, func(offset int) (FilePage, error) {
		return session.List(ctx, dirID, offset)
	}, func(page FilePage) (bool, error) {
		if !containsDirectory(page.Path, session.Source().Directory.ID) {
			return true, ErrFolderMoved
		}
		entries = append(entries, page.Files...)
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// SourceInfo reports one file, refusing entries outside the session's source.
func SourceInfo(ctx context.Context, session Session, id string) (File, error) {
	info, err := session.Info(ctx, id)
	if err != nil {
		return File{}, err
	}
	if !Within(info, session.Source()) {
		return File{}, ErrVideoGone
	}
	return info, nil
}

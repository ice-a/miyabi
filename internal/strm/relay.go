// Package strm resolves the fixed URLs written into exported .strm files to the
// media the library actually serves, so Emby clients stream through this server.
package strm

import (
	"context"
	"net/http"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/storage"
)

// publicPrefix is the stable path exported .strm files point at.
const publicPrefix = "/api/strm/play/"

// Relay resolves indexed video IDs into playable addresses.
type Relay struct {
	database *ent.Client
	storage  *storage.Service
}

// New requires an initialized database and storage service.
func New(database *ent.Client, mounted *storage.Service) *Relay {
	return &Relay{database: database, storage: mounted}
}

// StreamURL returns the address an Emby client should fetch for one file.
// Exported .strm files contain this server's own play path, so the relay only
// has to confirm the indexed file still exists inside the mounted directory.
func (relay *Relay) StreamURL(ctx context.Context, fileID, userAgent string) (string, error) {
	if _, err := relay.fileSource(ctx, fileID); err != nil {
		return "", err
	}
	return relay.address(fileID), nil
}

// Relay forwards one media request to the address StreamURL reported, so
// media servers receive the bytes and range metadata without a redirect.
func (relay *Relay) Relay(ctx context.Context, method, address string, headers http.Header) (*http.Response, error) {
	return relay.storage.Fetch(ctx, method, address, headers)
}

// address is the stable path served by the playback proxy.
func (relay *Relay) address(fileID string) string {
	return strings.TrimSuffix(publicPrefix, "/") + "/" + fileID
}

// fileSource reports the source of an indexed file, refusing files that left
// the mounted directory since the last scan.
func (relay *Relay) fileSource(ctx context.Context, fileID string) (domain.LibrarySource, error) {
	record, err := relay.database.File.Query().Where(file.FileIDEQ(fileID)).
		Select(file.FieldAccountID, file.FieldRootID).First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return domain.LibrarySource{}, domain.E(domain.KindNotFound, "视频文件已不在媒体库中，请重新扫描", err)
		}
		return domain.LibrarySource{}, domain.E(domain.KindUnexpected, "读取视频索引失败", err)
	}
	source := domain.LibrarySource{
		AccountID: record.AccountID,
		Directory: domain.LibraryDirectory{ID: record.RootID},
	}
	if _, err := relay.storage.OpenSource(ctx, source); err != nil {
		return domain.LibrarySource{}, err
	}
	return source, nil
}

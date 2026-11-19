package playback

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
)

// playBody releases the underlying body when the player disconnects, so an
// abandoned stream does not keep a file handle open.
type playBody struct {
	io.Reader
	io.Closer
	release func()
}

func (body *playBody) Close() error {
	err := body.Closer.Close()
	body.release()
	return err
}

// Stream serves one playable resource of a session. Indexed files are relayed
// byte-for-byte with range support; remote playlists are fetched, rewritten and
// relayed so every segment points back at this server.
func (service *Service) Stream(ctx context.Context, id string, index int, method string, headers http.Header) (*http.Response, error) {
	session, resource, err := service.resource(ctx, id, index)
	if err != nil {
		return nil, err
	}
	if resource.fileID != "" {
		return service.streamFile(ctx, session, resource, headers)
	}
	if !resource.playlist {
		// Direct media: let the browser stream from the origin.
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{resource.url.String()}},
			Body:       http.NoBody,
		}, nil
	}
	return service.streamPlaylist(ctx, session, resource, method, headers)
}

// streamFile relays one indexed file from the mounted directory.
func (service *Service) streamFile(ctx context.Context, session *playSession, resource playResource, headers http.Header) (*http.Response, error) {
	mounted, err := service.storage.OpenSource(ctx, session.source)
	if err != nil {
		return nil, err
	}
	response, err := mounted.OpenStream(ctx, resource.fileID, headers)
	if err != nil {
		return nil, err
	}
	if response == nil || response.Body == nil {
		return nil, domain.E(domain.KindNotFound, "视频已不在当前媒体目录中，请重新扫描", nil)
	}
	response.Body = &playBody{Reader: response.Body, Closer: response.Body, release: func() {}}
	return response, nil
}

// streamPlaylist relays one remote HLS playlist and rewrites its segment URIs.
func (service *Service) streamPlaylist(ctx context.Context, session *playSession, resource playResource, method string, headers http.Header) (*http.Response, error) {
	// A rewritten playlist is a new representation and cannot reuse upstream byte ranges.
	upstream, err := service.storage.Fetch(ctx, method, resource.url.String(), nil)
	if err != nil {
		return nil, err
	}
	body := &playBody{Reader: upstream.Body, Closer: upstream.Body, release: func() {}}
	switch upstream.StatusCode {
	case http.StatusOK, http.StatusPartialContent:
	case http.StatusRequestedRangeNotSatisfiable:
		body.Close()
		upstream.Body = http.NoBody
		return upstream, nil
	case http.StatusForbidden, http.StatusUnauthorized:
		body.Close()
		return nil, domain.E(domain.KindConflict, "播放地址已失效，请重新加载播放", nil)
	default:
		body.Close()
		return nil, domain.E(domain.KindUpstream, "视频流异常，请稍后重试", fmt.Errorf("upstream returned HTTP %d", upstream.StatusCode))
	}
	if method == http.MethodHead || upstream.StatusCode == http.StatusPartialContent {
		upstream.Body = body
		return upstream, nil
	}
	reader := bufio.NewReader(body)
	sniff, err := reader.Peek(len("#EXTM3U"))
	if err != nil || string(sniff) != "#EXTM3U" {
		body.Close()
		return nil, domain.E(domain.KindUpstream, "视频流异常，请稍后重试", fmt.Errorf("invalid HLS playlist: %w", err))
	}
	playlist, err := io.ReadAll(io.LimitReader(reader, maxPlaylistSize+1))
	body.Close()
	if err != nil {
		return nil, domain.E(domain.KindUpstream, "视频流异常，请稍后重试", fmt.Errorf("read playlist: %w", err))
	}
	if len(playlist) > maxPlaylistSize {
		return nil, domain.E(domain.KindUpstream, "视频流异常，请稍后重试", fmt.Errorf("playlist exceeds %d bytes", maxPlaylistSize))
	}
	service.mu.Lock()
	closed := session.ctx.Err() != nil
	service.mu.Unlock()
	if closed {
		return nil, domain.E(domain.KindCanceled, "播放会话已结束，请重新播放", context.Canceled)
	}
	rewritten, err := rewritePlaylist(playlist, resource.url, session.register)
	if err != nil {
		return nil, err
	}
	upstream.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
	upstream.Header.Set("Content-Type", "application/vnd.apple.mpegurl")
	upstream.Header.Del("Content-Range")
	upstream.Header.Del("Accept-Ranges")
	upstream.Header.Del("ETag")
	upstream.Header.Del("Last-Modified")
	upstream.Header.Del("Content-Encoding")
	upstream.Body = io.NopCloser(strings.NewReader(rewritten))
	return upstream, nil
}

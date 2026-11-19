package playback

import (
	"context"
	"io/fs"
	"net/url"
	"strconv"

	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/domain"
)

// playResource is one address handed to the browser. Local files are addressed
// by their indexed file ID; remote playlists are addressed by their URL.
type playResource struct {
	fileID   string
	url      *url.URL
	playlist bool
}

type mediaSpec struct {
	fileID    string
	address   string
	mediaType string
	label     string
	playlist  bool
}

type playSession struct {
	id        string
	source    domain.LibrarySource
	ctx       context.Context
	cancel    context.CancelFunc
	timer     sessTimer
	resources []playResource
	byAddress map[string]int
}

// sessTimer only needs to stop the abandoned-session timer.
type sessTimer interface {
	Stop() bool
}

func (service *Service) createSession(source domain.LibrarySource, specs []mediaSpec) (Playback, error) {
	session := &playSession{id: uuid.NewString(), source: source, byAddress: make(map[string]int)}
	result := Playback{ID: session.id, Source: source, Sources: make([]MediaSource, 0, len(specs))}
	for _, spec := range specs {
		local, err := session.register(spec)
		if err != nil {
			return Playback{}, err
		}
		result.Sources = append(result.Sources, MediaSource{Src: local, Type: spec.mediaType, Label: spec.label})
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	session.ctx, session.cancel = context.WithCancel(context.Background())
	service.sessions[session.id] = session
	// Explicit close releases immediately; the timer clears sessions abandoned by a closed tab.
	session.timer = service.afterFunc(service.sessionTTL, func() { service.Release(session.id) })
	return result, nil
}

// register turns a media spec into an opaque local stream path. The browser
// never receives upstream URLs, only paths served by this handler.
func (session *playSession) register(spec mediaSpec) (string, error) {
	key := spec.address
	if spec.fileID != "" {
		key = "file:" + spec.fileID
	}
	index, exists := session.byAddress[key]
	if !exists {
		index = len(session.resources)
		session.byAddress[key] = index
		resource := playResource{fileID: spec.fileID, playlist: spec.playlist}
		if spec.fileID == "" {
			address, err := url.Parse(spec.address)
			if err != nil || (address.Scheme != "https" && address.Scheme != "http") ||
				address.Host == "" || address.User != nil {
				return "", ErrInvalidMediaURL
			}
			resource.url = address
		}
		session.resources = append(session.resources, resource)
	} else if spec.playlist {
		session.resources[index].playlist = true
	}
	return "/api/play/" + session.id + "/stream/" + strconv.Itoa(index), nil
}

func (service *Service) Release(id string) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if session, exists := service.sessions[id]; exists {
		session.timer.Stop()
		session.cancel()
		delete(service.sessions, id)
	}
}

func (service *Service) resource(ctx context.Context, id string, index int) (*playSession, playResource, error) {
	service.mu.Lock()
	session, exists := service.sessions[id]
	if !exists || index < 0 || index >= len(session.resources) {
		service.mu.Unlock()
		return nil, playResource{}, domain.E(domain.KindNotFound, "播放会话已结束，请重新播放", fs.ErrNotExist)
	}
	resource := session.resources[index]
	service.mu.Unlock()

	if _, err := service.storage.OpenSource(ctx, session.source); err != nil {
		service.Release(id)
		return nil, playResource{}, domain.E(domain.KindNotFound, "媒体目录已变更，请重新播放", fs.ErrNotExist)
	}
	return session, resource, nil
}

// ErrInvalidMediaURL reports a media address the proxy refuses to relay.
var ErrInvalidMediaURL = domain.E(domain.KindInvalid, "播放地址不受支持", nil)

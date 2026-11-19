package playback

import (
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/storage"
)

type PlayFiles struct {
	Code   string               `json:"code"`
	Title  string               `json:"title"`
	Files  []domain.LibraryFile `json:"files"`
	Source domain.LibrarySource `json:"source"`
}

type MediaSource struct {
	Src   string `json:"src"`
	Type  string `json:"type"`
	Label string `json:"label"`
}

type Playback struct {
	ID      string               `json:"id"`
	FileID  string               `json:"file_id"`
	Sources []MediaSource        `json:"sources"`
	Source  domain.LibrarySource `json:"source"`
	Resume  *domain.WatchResume  `json:"resume,omitempty"`
}

// Service manages playback sessions and stream proxying.
type Service struct {
	database *ent.Client
	storage  *storage.Service
	mu       sync.Mutex
	sessions map[string]*playSession
	// sessionTTL releases sessions abandoned by a closed tab.
	sessionTTL time.Duration
	// afterFunc schedules session expiry; tests replace it with a fake.
	afterFunc func(time.Duration, func()) sessTimer
}

func New(database *ent.Client, mounted *storage.Service, sessionTTL time.Duration) *Service {
	return &Service{
		database:   database,
		storage:    mounted,
		sessions:   make(map[string]*playSession),
		sessionTTL: sessionTTL,
		afterFunc: func(delay time.Duration, fn func()) sessTimer {
			return time.AfterFunc(delay, fn)
		},
	}
}

func (service *Service) Close() {
	service.mu.Lock()
	defer service.mu.Unlock()
	for id, session := range service.sessions {
		session.timer.Stop()
		session.cancel()
		delete(service.sessions, id)
	}
}

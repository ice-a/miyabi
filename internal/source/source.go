package source

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
)

const settingKey = "sources"

// Kind identifies how a storage source is reached.
type Kind string

const (
	KindLocal Kind = "local"
	// KindLegacyPan is the retired cloud drive kind. Saved configurations that
	// still reference it are reported as unsupported instead of being loaded.
	KindLegacyPan Kind = "115"
)

// Source is a configurable storage location.
type Source struct {
	ID      string            `json:"id"`
	Kind    Kind              `json:"kind"`
	Name    string            `json:"name"`
	Enabled bool              `json:"enabled"`
	Config  map[string]string `json:"config,omitempty"`
}

// Service persists the configured storage sources as a single settings row.
type Service struct {
	client      *ent.Client
	onLocalScan func(context.Context, string, bool) error
}

// LocalTarget is an enabled local source directory and its metadata mode.
type LocalTarget struct {
	Path         string
	MetadataOnly bool
}

func New(client *ent.Client) *Service {
	return &Service{client: client}
}

// SetLocalScanHandler lets the composition root enqueue scans after sources change.
func (s *Service) SetLocalScanHandler(handler func(context.Context, string, bool) error) {
	s.onLocalScan = handler
}

func (s *Service) List(ctx context.Context) ([]Source, error) {
	sources, _, err := database.LoadSetting[[]Source](ctx, s.client, settingKey)
	if err != nil {
		return nil, err
	}
	if sources == nil {
		sources = []Source{}
	}
	return sources, nil
}

func (s *Service) Replace(ctx context.Context, sources []Source) ([]Source, error) {
	if sources == nil {
		sources = []Source{}
	}
	for i := range sources {
		if strings.TrimSpace(sources[i].ID) == "" {
			sources[i].ID = uuid.NewString()
		}
		if err := validate(sources[i]); err != nil {
			return nil, err
		}
	}
	if err := database.SaveSetting(ctx, s.client, settingKey, sources); err != nil {
		return nil, err
	}
	s.triggerScans(ctx, sources)
	return sources, nil
}

func (s *Service) triggerScans(ctx context.Context, sources []Source) {
	for _, src := range sources {
		if !src.Enabled || src.Kind != KindLocal || s.onLocalScan == nil {
			continue
		}
		path := strings.TrimSpace(src.Config["path"])
		if path == "" {
			continue
		}
		if err := s.onLocalScan(ctx, path, localMetadataOnly(src)); err != nil {
			slog.WarnContext(ctx, "failed to enqueue local scan", "source", src.Name, "path", path, "error", err)
		}
	}
}

// LocalTargets returns the enabled local source directories with their mode.
func (s *Service) LocalTargets(ctx context.Context) ([]LocalTarget, error) {
	sources, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	targets := make([]LocalTarget, 0)
	for _, src := range sources {
		if src.Kind != KindLocal || !src.Enabled {
			continue
		}
		if path := strings.TrimSpace(src.Config["path"]); path != "" {
			targets = append(targets, LocalTarget{Path: path, MetadataOnly: localMetadataOnly(src)})
		}
	}
	return targets, nil
}

func localMetadataOnly(src Source) bool {
	return strings.TrimSpace(src.Config["scrape_mode"]) == "local"
}

// LocalRoots returns the enabled local source directories, for startup scanning.
func (s *Service) LocalRoots(ctx context.Context) ([]string, error) {
	targets, err := s.LocalTargets(ctx)
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0, len(targets))
	for _, target := range targets {
		roots = append(roots, target.Path)
	}
	return roots, nil
}

func validate(src Source) error {
	if strings.TrimSpace(src.Name) == "" {
		return domain.E(domain.KindInvalid, "存储源名称不能为空", nil)
	}
	switch src.Kind {
	case KindLocal:
		path := strings.TrimSpace(src.Config["path"])
		if path == "" {
			return domain.E(domain.KindInvalid, "本地存储源需要目录路径", nil)
		}
		info, err := os.Stat(path)
		if err != nil {
			return domain.E(domain.KindInvalid, fmt.Sprintf("目录不可访问：%s", path), nil)
		}
		if !info.IsDir() {
			return domain.E(domain.KindInvalid, fmt.Sprintf("路径不是目录：%s", path), nil)
		}
	case KindLegacyPan:
		// The cloud drive integration was removed; keeping the stored entry
		// readable lets users replace it with a local source explicitly.
		return domain.E(domain.KindInvalid, "115 存储源已下线，请改用本地目录存储源", nil)
	default:
		return domain.E(domain.KindInvalid, "未知的存储源类型", nil)
	}
	return nil
}

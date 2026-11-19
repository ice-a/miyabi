package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return New(store.Client)
}

func TestReplaceValidates(t *testing.T) {
	ctx := t.Context()
	service := newTestService(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		source Source
	}{
		{"empty name", Source{ID: "1", Kind: KindLocal, Name: " ", Config: map[string]string{"path": dir}}},
		{"unknown kind", Source{ID: "1", Kind: "ftp", Name: "x"}},
		{"local missing path", Source{ID: "1", Kind: KindLocal, Name: "x"}},
		{"local not directory", Source{ID: "1", Kind: KindLocal, Name: "x", Config: map[string]string{"path": file}}},
		{"local missing directory", Source{ID: "1", Kind: KindLocal, Name: "x", Config: map[string]string{"path": filepath.Join(dir, "nope")}}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := service.Replace(ctx, []Source{testCase.source}); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestReplaceAssignsIDAndPersists(t *testing.T) {
	ctx := t.Context()
	service := newTestService(t)
	dir := t.TempDir()

	saved, err := service.Replace(ctx, []Source{
		{Kind: KindLocal, Name: "Local", Enabled: true, Config: map[string]string{"path": dir}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved[0].ID == "" {
		t.Fatal("expected an assigned id")
	}
	list, err := service.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != saved[0].ID {
		t.Fatalf("list = %+v", list)
	}
}

func TestLocalRoots(t *testing.T) {
	ctx := t.Context()
	service := newTestService(t)
	first := t.TempDir()
	second := t.TempDir()

	if _, err := service.Replace(ctx, []Source{
		{Kind: KindLocal, Name: "first", Enabled: true, Config: map[string]string{"path": first}},
		{Kind: KindLocal, Name: "second", Enabled: false, Config: map[string]string{"path": second}},
	}); err != nil {
		t.Fatal(err)
	}

	roots, err := service.LocalRoots(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || roots[0] != first {
		t.Fatalf("roots = %v", roots)
	}
}

func TestRetiredCloudKind(t *testing.T) {
	ctx := t.Context()
	service := newTestService(t)

	if _, err := service.Replace(ctx, []Source{
		{Kind: KindLegacyPan, Name: "cloud", Enabled: true, Config: map[string]string{"account_id": "A"}},
	}); err == nil {
		t.Fatal("expected the retired cloud kind to be rejected")
	}
}

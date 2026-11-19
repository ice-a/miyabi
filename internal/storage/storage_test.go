package storage

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
)

// newTestService mounts dir as the media root without a database.
func newTestService(t *testing.T, dir string) *Service {
	t.Helper()
	service := &Service{}
	service.mounted = mountRecord{Directory: domain.LibraryDirectory{ID: dir, Name: filepath.Base(dir), Path: dir}}
	service.root = dir
	return service
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestOpenStreamServesByteRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "movie.mp4")
	writeFile(t, path, "0123456789")

	service := newTestService(t, dir)
	session, err := service.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	headers := http.Header{"Range": {"bytes=2-5"}}
	response, err := session.OpenStream(context.Background(), path, headers)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", response.StatusCode)
	}
	if got := response.Header.Get("Content-Range"); got != "bytes 2-5/10" {
		t.Fatalf("Content-Range = %q", got)
	}
	if got := response.Header.Get("Content-Length"); got != "4" {
		t.Fatalf("Content-Length = %q", got)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "2345" {
		t.Fatalf("body = %q, want 2345", body)
	}
}

func TestOpenStreamRejectsEscapingPaths(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.mp4")
	writeFile(t, outside, "secret")

	service := newTestService(t, dir)
	session, err := service.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.OpenStream(context.Background(), outside, nil); err == nil {
		t.Fatal("expected an error for a path outside the mount")
	}
	if _, err := session.OpenStream(context.Background(), filepath.Join(dir, "..", "secret.mp4"), nil); err == nil {
		t.Fatal("expected an error for a relative escape")
	}
}

func TestByteRangeSuffixAndUnsatisfiable(t *testing.T) {
	for _, tc := range []struct {
		value string
		ok    bool
		start int64
		size  int64
	}{
		{"bytes=0-9", true, 0, 10},
		{"bytes=5-", true, 5, 5},
		{"bytes=-5", false, 0, 0},
		{"bytes=20-30", false, 0, 0},
		{"", false, 0, 0},
	} {
		start, length, ok := byteRange(tc.value, 10)
		if ok != tc.ok || start != tc.start || length != tc.size {
			t.Fatalf("%q -> (%d, %d, %v), want (%d, %d, %v)", tc.value, start, length, ok, tc.start, tc.size, tc.ok)
		}
	}
}

func TestMountRejectsMissingDirectory(t *testing.T) {
	dir := t.TempDir()
	store, err := database.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := New(context.Background(), store.Client, nil)
	if _, err := service.Mount(context.Background(), domain.LibraryDirectory{ID: filepath.Join(dir, "missing")}); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}

func TestMediaTypeCoversCommonVideoContainers(t *testing.T) {
	for name, want := range map[string]string{
		"movie.mp4":  "video/mp4",
		"movie.mkv":  "video/x-matroska",
		"movie.avi":  "video/x-msvideo",
		"movie.strm": "text/plain; charset=utf-8",
	} {
		if got := MediaType(name); got != want {
			t.Fatalf("MediaType(%q) = %q, want %q", name, got, want)
		}
	}
}

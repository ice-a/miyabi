package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// newSTRMRouter mounts the relay route without an access gate.
func newSTRMRouter(t *testing.T, stub *strmRelayStub) http.Handler {
	t.Helper()
	return NewRouter(Dependencies{
		Access: NewAccessGateService("", ""),
		STRM:   stub,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

// An escaped absolute path is a single path segment; the handler must hand the
// decoded file ID to the relay and stream the body instead of redirecting.
func TestSTRMStreamHandlerDecodesEscapedFileIDAndStreamsBody(t *testing.T) {
	const fileID = `C:\Movies\ABP-001.mp4`
	stub := &strmRelayStub{
		streamURL:    "/api/strm/play/" + url.PathEscape(fileID),
		body:         "media-bytes",
		relayHeaders: http.Header{"Content-Type": {"video/mp4"}, "Accept-Ranges": {"bytes"}},
	}
	router := newSTRMRouter(t, stub)

	request := httptest.NewRequest(http.MethodGet, "/api/strm/play/"+url.PathEscape(fileID), nil)
	request.Header.Set("Range", "bytes=10-20")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if stub.lastFileID != fileID {
		t.Fatalf("file id = %q, want %q", stub.lastFileID, fileID)
	}
	if stub.lastMethod != http.MethodGet {
		t.Fatalf("upstream method = %q, want GET", stub.lastMethod)
	}
	if response.Body.String() != "media-bytes" {
		t.Fatalf("body = %q, want media-bytes", response.Body.String())
	}
	if got := response.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Fatalf("Accept-Ranges = %q, want bytes", got)
	}
}

// HEAD probes must forward metadata without a body.
func TestSTRMStreamHandlerForwardsHEADMetadata(t *testing.T) {
	stub := &strmRelayStub{
		streamURL:    "/api/strm/play/123",
		relayHeaders: http.Header{"Content-Type": {"video/mp4"}, "Content-Length": {"104857600"}},
	}
	router := newSTRMRouter(t, stub)

	request := httptest.NewRequest(http.MethodHead, "/api/strm/play/123", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if stub.lastMethod != http.MethodHead {
		t.Fatalf("upstream method = %q, want HEAD", stub.lastMethod)
	}
	if got := response.Header().Get("Content-Length"); got != "104857600" {
		t.Fatalf("Content-Length = %q, want 104857600", got)
	}
	if response.Body.Len() != 0 {
		t.Fatalf("HEAD returned %d body bytes", response.Body.Len())
	}
}

// The STRM token guards the route whether or not a session is signed in.
func TestSTRMStreamHandlerRequiresToken(t *testing.T) {
	stub := &strmRelayStub{streamURL: "/api/strm/play/123"}
	router := NewRouter(Dependencies{
		Access:    NewAccessGateService("", ""),
		STRM:      stub,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		STRMToken: "secret123",
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/strm/play/123", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	if stub.relayCalls != 0 {
		t.Fatalf("relay called %d times for a rejected request", stub.relayCalls)
	}
}

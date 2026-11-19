package api

import (
	"context"
	"io"
	"net/http"
	"strings"
)

type strmRelayStub struct {
	streamURL    string
	streamErr    error
	body         string
	relayHeaders http.Header
	relayStatus  int
	relayErr     error
	relayCalls   int
	lastFileID   string
	lastMethod   string
}

func (s *strmRelayStub) StreamURL(ctx context.Context, fileID, userAgent string) (string, error) {
	s.lastFileID = fileID
	if s.streamErr != nil {
		return "", s.streamErr
	}
	return s.streamURL, nil
}

func (s *strmRelayStub) Relay(ctx context.Context, method, address string, headers http.Header) (*http.Response, error) {
	s.relayCalls++
	s.lastMethod = method
	if s.relayErr != nil {
		return nil, s.relayErr
	}
	res := &http.Response{
		StatusCode: s.relayStatus,
		Header:     s.relayHeaders,
		Body:       io.NopCloser(strings.NewReader(s.body)),
	}
	if res.Header == nil {
		res.Header = make(http.Header)
	}
	if res.StatusCode == 0 {
		res.StatusCode = http.StatusOK
	}
	return res, nil
}

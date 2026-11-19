package storage

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
)

// remoteClient is the fallback transport for services built without an
// explicit HTTP client.
var remoteClient = &http.Client{Timeout: remoteTimeout}

// serveFile builds a response streaming file, translating the indexed file
// size into a Content-Length and honouring a single byte range so video players
// can seek without downloading the whole file again.
func serveFile(file *os.File, info os.FileInfo, headers http.Header) (*http.Response, error) {
	size := info.Size()
	response := &http.Response{
		StatusCode:    http.StatusOK,
		Header:        make(http.Header),
		ContentLength: size,
		Body:          &fileBody{file: file},
	}
	response.Header.Set("Content-Type", MediaType(filepath.Base(file.Name())))
	response.Header.Set("Accept-Ranges", "bytes")
	response.Header.Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))
	if start, length, ok := byteRange(headers.Get("Range"), size); ok {
		if _, err := file.Seek(start, 0); err != nil {
			return nil, err
		}
		response.StatusCode = http.StatusPartialContent
		response.ContentLength = length
		response.Header.Set("Content-Range", "bytes "+strconv.FormatInt(start, 10)+"-"+
			strconv.FormatInt(start+length-1, 10)+"/"+strconv.FormatInt(size, 10))
		response.Body = &fileBody{file: file, remaining: length}
	}
	response.Header.Set("Content-Length", strconv.FormatInt(response.ContentLength, 10))
	return response, nil
}

// byteRange parses a single "bytes=first-last" range. Unsatisfiable or
// multipart requests fall back to the whole file, which is what players expect
// from a progressive download endpoint.
func byteRange(value string, size int64) (start, length int64, ok bool) {
	const prefix = "bytes="
	if size <= 0 || !strings.HasPrefix(value, prefix) {
		return 0, 0, false
	}
	spec := value[len(prefix):]
	separator := strings.IndexByte(spec, '-')
	if separator <= 0 || strings.ContainsRune(spec, ',') {
		return 0, 0, false
	}
	first, err := strconv.ParseInt(spec[:separator], 10, 64)
	if err != nil || first < 0 || first >= size {
		return 0, 0, false
	}
	last := size - 1
	if tail := spec[separator+1:]; tail != "" {
		parsed, err := strconv.ParseInt(tail, 10, 64)
		if err != nil {
			return 0, 0, false
		}
		last = min(parsed, size-1)
	}
	if last < first {
		return 0, 0, false
	}
	return first, last - first + 1, true
}

// fileBody streams a local file and closes it once exhausted.
type fileBody struct {
	file      *os.File
	remaining int64
	closed    bool
}

func (body *fileBody) Read(buffer []byte) (int, error) {
	if body.remaining <= 0 {
		return 0, io.EOF
	}
	if int64(len(buffer)) > body.remaining {
		buffer = buffer[:body.remaining]
	}
	count, err := body.file.Read(buffer)
	body.remaining -= int64(count)
	if err != nil {
		_ = body.Close()
	}
	return count, err
}

func (body *fileBody) Close() error {
	if body.closed {
		return nil
	}
	body.closed = true
	return body.file.Close()
}

// MediaType maps the media extensions the library indexes onto the content
// types players expect.
func MediaType(name string) string {
	switch filepath.Ext(name) {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".mkv":
		return "video/x-matroska"
	case ".avi":
		return "video/x-msvideo"
	case ".wmv":
		return "video/x-ms-wmv"
	case ".mov":
		return "video/quicktime"
	case ".ts":
		return "video/mp2t"
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".strm":
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// Fetch relays a remote media address. Remote playlists appear when an
// indexed .strm file points at an HTTP source, so the proxy needs the same
// transport guarantees as the rest of the downloader.
func (service *Service) Fetch(ctx context.Context, method, address string, headers http.Header) (*http.Response, error) {
	parsed, err := url.Parse(address)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return nil, domain.E(domain.KindInvalid, "播放地址无效", nil)
	}
	request, err := http.NewRequestWithContext(ctx, method, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", UserAgent)
	for name, values := range headers {
		if http.CanonicalHeaderKey(name) == "User-Agent" {
			continue
		}
		request.Header[name] = append([]string(nil), values...)
	}
	client := service.client
	if client == nil {
		client = remoteClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, domain.E(domain.KindUpstream, "视频流异常，请稍后重试", err)
	}
	return response, nil
}

// openWithin opens a regular file that must live inside root.
func openWithin(root, path string) (*os.File, os.FileInfo, error) {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." || strings.HasPrefix(relative, "..") {
		return nil, nil, ErrNotStreamable
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, ErrNotStreamable
	}
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		file.Close()
		return nil, nil, ErrNotStreamable
	}
	return file, info, nil
}

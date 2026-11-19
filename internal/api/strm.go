package api

import (
	"context"
	"crypto/subtle"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
)

// STRMRelay resolves the fixed URLs written into exported .strm files.
type STRMRelay interface {
	// StreamURL returns the address the relay serves for one indexed file.
	StreamURL(ctx context.Context, fileID, userAgent string) (string, error)
	// Relay forwards one media request to address, preserving its method,
	// headers and body so byte ranges reach the reader unchanged.
	Relay(ctx context.Context, method, address string, headers http.Header) (*http.Response, error)
}

// strmStreamHandler resolves an exported .strm file ID and relays the stream.
// Media servers cannot follow the stable play path themselves, so the body is
// copied through this server with the same status and range metadata.
func strmStreamHandler(relay STRMRelay, token string, gate AccessGate) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[struct {
			FileID string `uri:"fileID" binding:"required,max=4096"`
		}](c)
		if !ok {
			return
		}
		if !strmAuthorized(c, token, gate) {
			c.Error(domain.E(domain.KindUnauthorized, "无效的播放令牌", nil))
			return
		}
		// A wildcard segment arrives with its leading slash; the ID itself never
		// starts with one.
		fileID := strings.TrimPrefix(uri.FileID, "/")
		if decoded, err := url.PathUnescape(fileID); err == nil {
			fileID = decoded
		}
		streamURL, err := relay.StreamURL(c.Request.Context(), fileID, c.Request.UserAgent())
		if err != nil {
			c.Error(err)
			return
		}
		response, err := relay.Relay(c.Request.Context(), c.Request.Method, streamURL, c.Request.Header)
		if err != nil {
			c.Error(err)
			return
		}
		defer response.Body.Close()
		for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
			if value := response.Header.Get(name); value != "" {
				c.Header(name, value)
			}
		}
		if c.Writer.Header().Get("Content-Type") == "" || c.Writer.Header().Get("Content-Type") == "application/octet-stream" {
			c.Header("Content-Type", "video/mp4")
		}
		c.Status(response.StatusCode)
		if c.Request.Method == http.MethodHead {
			return
		}
		if _, err := io.Copy(c.Writer, response.Body); err != nil {
			c.Error(domain.E(domain.KindCanceled, "媒体流中断", err))
		}
	}
}

// Media servers cannot sign in, so the configured STRM token authorizes a
// request on its own. A signed-in session is accepted as well; without a
// token or access password the relay is open.
func strmAuthorized(c *gin.Context, token string, gate AccessGate) bool {
	if token != "" && subtle.ConstantTimeCompare([]byte(c.Query("token")), []byte(token)) == 1 {
		return true
	}
	if gate.Enabled() {
		session := extractToken(c)
		if session == "" {
			return false
		}
		err := gate.VerifyToken(session)
		return err == nil
	}
	return token == ""
}

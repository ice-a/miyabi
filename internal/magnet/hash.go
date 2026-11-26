package magnet

import (
	"encoding/base32"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
)

var (
	btihHexRegex    = regexp.MustCompile(`(?i)^[0-9a-f]{40}$`)
	btihBase32Regex = regexp.MustCompile(`(?i)^[a-z2-7]{32}$`)
	btihParamRegex  = regexp.MustCompile(`(?i)xt=urn:btih:([0-9a-z]+)`)
)

// ParseHash extracts the BitTorrent info hash from a magnet URI (btih hex or
// base32 form) or accepts a bare hash. The result is lowercase hex; a base32
// hash is decoded to its 20-byte hex form.
func ParseHash(input string) (string, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return "", domain.E(domain.KindInvalid, "磁力链接不能为空", nil)
	}
	candidate := raw
	if matches := btihParamRegex.FindStringSubmatch(raw); matches != nil {
		candidate = matches[1]
	}
	candidate = strings.ToLower(candidate)
	switch {
	case btihHexRegex.MatchString(candidate):
		return candidate, nil
	case btihBase32Regex.MatchString(candidate):
		decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(candidate))
		if err != nil || len(decoded) != 20 {
			return "", domain.E(domain.KindInvalid, "无法识别磁力链接中的资源哈希", err)
		}
		return hex.EncodeToString(decoded), nil
	default:
		return "", domain.E(domain.KindInvalid, "无法识别磁力链接中的资源哈希", nil)
	}
}

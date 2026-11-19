package providers

import (
	"context"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/net/html"
)

const missavBase = "https://missav.ai/"

// MissAV adapts the MissAV movie page into the metadata provider contract.
//
// Selectors follow the publicly documented MissAV page layout (Tailwind based,
// h1.text-base.text-secondary title, span.text-secondary field pairs,
// meta og:image cover). The site was unreachable from this environment during
// development, so the layout is reproduced from third-party scrapers
// (gakkiyomi/MissAV-API, Cat-Ling/missAV_api) and has not been verified live.
type MissAV struct{ client }

func NewMissAV(proxy *netx.ProxyManager) *MissAV {
	return &MissAV{newClient(proxy, missavBase, "", "missav.ai", "missav.ws", "missav.com", "dmm.co.jp")}
}

func (*MissAV) ID() string { return "missav" }

func (*MissAV) Supports(code string) bool {
	upper := strings.ToUpper(code)
	return upper != "" && !strings.HasPrefix(upper, "FC2") && !strings.HasPrefix(upper, "HEYZO")
}

func (s *MissAV) Media(ctx context.Context, target string) (domain.Media, error) {
	return s.client.Media(ctx, target)
}

func (s *MissAV) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	if code == "" || !s.Supports(code) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	body, err := s.get(ctx, missavBase+"cn/"+strings.ToLower(code))
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	return parseMissAV(string(body), code)
}

func parseMissAV(body, code string) (domain.MovieMetadata, error) {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	// MissAV lazy-loads artwork and exposes the canonical cover via og:image.
	cover := ""
	for _, meta := range nodes(doc, func(n *html.Node) bool {
		return n.Data == "meta" && attr(n, "property") == "og:image"
	}) {
		if url := attr(meta, "content"); strings.HasPrefix(url, "https://") {
			cover = url
			break
		}
	}

	// The detail metadata block repeats "<span>label:</span><span>value</span>"
	// pairs (span.text-secondary labels, span.font-medium values).
	field := func(label string) string {
		return missAVField(doc, label)
	}
	identity := codeid.Normalize(field("識別碼"))
	if identity == "" || identity != code {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}

	title := strings.TrimSpace(strings.TrimPrefix(text(tag(doc, "h1")), code))

	movie := domain.Movie{
		Code:        code,
		Title:       title,
		ReleaseDate: releaseDate(field("發行日期")),
		Duration:    integer(field("長度")),
		Cover:       cover,
		Sources:     []domain.SourceID{{Provider: "missav", ID: code}},
	}
	movie.Thumbnail = movie.Cover
	seenActors := map[string]bool{}
	for _, anchor := range missAVAnchors(doc, "演員") {
		name := strings.TrimSpace(text(anchor))
		if name == "" {
			continue
		}
		id := strings.TrimSpace(attr(anchor, "href"))
		if index := strings.LastIndex(id, "/"); index >= 0 {
			id = id[index+1:]
		}
		if id == "" {
			id = name
		}
		if seenActors[id] {
			continue
		}
		seenActors[id] = true
		movie.Actors = append(movie.Actors, domain.Actor{Provider: "missav", ID: id, Name: name})
	}
	seenTags := map[string]bool{}
	for _, anchor := range missAVAnchors(doc, "類別") {
		name := strings.TrimSpace(text(anchor))
		if name == "" {
			continue
		}
		id := strings.TrimSpace(attr(anchor, "href"))
		if index := strings.LastIndex(id, "/"); index >= 0 {
			id = id[index+1:]
		}
		if id == "" {
			id = name
		}
		if seenTags[id] {
			continue
		}
		seenTags[id] = true
		movie.Tags = append(movie.Tags, domain.Tag{Provider: "missav", ID: id, Name: name})
	}

	result := domain.MovieMetadata{Detail: domain.MovieDetail{Movie: movie}}
	if movie.Cover != "" {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "missav", URL: movie.Cover, Role: "cover"})
	}
	return result, nil
}

// missAVField scans span label/value pairs for the requested traditional
// Chinese label (with either a full-width or ASCII colon).
func missAVField(root *html.Node, label string) string {
	for _, span := range nodes(root, func(n *html.Node) bool { return n.Data == "span" && hasClass(n, "text-secondary") }) {
		if value := missAVLabelValue(span, label); value != "" {
			return value
		}
	}
	for _, span := range nodes(root, func(n *html.Node) bool { return n.Data == "span" }) {
		if hasClass(span, "text-secondary") {
			continue
		}
		if value := missAVLabelValue(span, label); value != "" {
			return value
		}
	}
	return ""
}

func missAVLabelValue(span *html.Node, label string) string {
	head := strings.TrimSpace(text(span))
	// The label may occupy the whole span ("label:") or share the span with
	// its value ("label: value").
	before, after, found := strings.Cut(head, ":")
	if !found {
		before, after, found = strings.Cut(head, "：")
	}
	if !found || strings.TrimSpace(before) != label {
		return ""
	}
	// The value continues inside the same span ("label: value") or in the
	// following sibling span / anchors.
	if rest := strings.TrimSpace(after); rest != "" {
		return rest
	}
	for sibling := span.NextSibling; sibling != nil; sibling = sibling.NextSibling {
		if sibling.Type != html.ElementNode {
			continue
		}
		// Skip sibling spans that start another label/value pair.
		siblingText := strings.TrimSpace(text(sibling))
		if strings.HasSuffix(siblingText, ":") || strings.HasSuffix(siblingText, "：") {
			return ""
		}
		if siblingText != "" {
			return siblingText
		}
	}
	return ""
}

// missAVAnchors collects the anchors that follow a field label span, stopping
// at the next label span so neighbouring fields do not bleed in.
func missAVAnchors(root *html.Node, label string) []*html.Node {
	isLabel := func(n *html.Node) bool {
		if n == nil || n.Type != html.ElementNode || n.Data != "span" {
			return false
		}
		tail := strings.TrimSpace(text(n))
		return strings.HasSuffix(tail, ":") || strings.HasSuffix(tail, "：")
	}
	for _, span := range nodes(root, func(n *html.Node) bool { return n.Data == "span" && hasClass(n, "text-secondary") }) {
		head := strings.TrimSpace(text(span))
		trimmed := strings.TrimSuffix(strings.TrimSuffix(head, ":"), "：")
		if strings.TrimSpace(trimmed) != label {
			continue
		}
		var anchors []*html.Node
		for sibling := span.NextSibling; sibling != nil; sibling = sibling.NextSibling {
			if isLabel(sibling) {
				break
			}
			if sibling.Type == html.ElementNode && sibling.Data == "a" {
				anchors = append(anchors, sibling)
				continue
			}
			if sibling.Type != html.ElementNode {
				continue
			}
			anchors = append(anchors, nodes(sibling, func(n *html.Node) bool { return n.Data == "a" })...)
		}
		return anchors
	}
	return nil
}

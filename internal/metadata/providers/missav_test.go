package providers

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
)

func TestParseMissAVFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "missav_ssis-001.html"))
	if err != nil {
		t.Fatal(err)
	}

	result, err := parseMissAV(string(raw), "SSIS-001")
	if err != nil {
		t.Fatal(err)
	}
	movie := result.Detail.Movie
	if movie.Code != "SSIS-001" || movie.ReleaseDate != "2021-02-18" || movie.Duration != 147 {
		t.Fatalf("scalars = %+v", movie)
	}
	// The h1 repeats the code as prefix and trails the lead actress name.
	if movie.Title != "封面太顶了，无论看多少次都会心动。 乃木蛍" {
		t.Fatalf("title = %q", movie.Title)
	}
	if len(movie.Actors) != 2 || movie.Actors[0].Name != "向井藍" || movie.Actors[0].ID != "ai-mukai" {
		t.Fatalf("actors = %+v", movie.Actors)
	}
	if len(movie.Tags) != 2 || movie.Tags[0].Name != "多P" {
		t.Fatalf("tags = %+v", movie.Tags)
	}
	if movie.Cover != "https://pics.dmm.co.jp/digital/video/ssis00001/ssis00001pl.jpg" {
		t.Fatalf("cover = %q", movie.Cover)
	}
	if len(result.Images) != 1 || result.Images[0].Role != "cover" || result.Images[0].Provider != "missav" {
		t.Fatalf("images = %+v", result.Images)
	}
}

func TestParseMissAVInlineSharedSpanValue(t *testing.T) {
	body := `<html><head>
<meta property="og:image" content="https://pics.dmm.co.jp/digital/video/ssis00001/ssis00001pl.jpg">
</head><body>
<h1 class="text-base text-secondary md:text-lg font-semibold">SSIS-001 封面太顶了，无论看多少次都会心动。</h1>
<div>
<span class="text-secondary">識別碼: ssis-001</span>
<span class="text-secondary">發行日期: 2021-02-18</span>
<span class="text-secondary">長度: 147分钟</span>
</div>
<div>
<span class="text-secondary">演員:</span>
<a href="https://missav.ai/cn/actress/tsukasa-aoi">葵つかさ</a>
<a href="https://missav.ai/cn/actress/sayaka-otoshiro">乙白さやか</a>
</div>
<div>
<span class="text-secondary">類別:</span>
<a href="https://missav.ai/cn/genre/30">美少女</a>
<a href="https://missav.ai/cn/genre/4o">高畫質</a>
</div>
</body></html>`

	result, err := parseMissAV(body, "SSIS-001")
	if err != nil {
		t.Fatal(err)
	}
	movie := result.Detail.Movie
	if movie.Code != "SSIS-001" || movie.ReleaseDate != "2021-02-18" || movie.Duration != 147 {
		t.Fatalf("scalars = %+v", movie)
	}
	if movie.Title != "封面太顶了，无论看多少次都会心动。" {
		t.Fatalf("title = %q", movie.Title)
	}
	if len(movie.Actors) != 2 || movie.Actors[0].Name != "葵つかさ" || movie.Actors[0].ID != "tsukasa-aoi" {
		t.Fatalf("actors = %+v", movie.Actors)
	}
	if len(movie.Tags) != 2 || movie.Tags[0].Name != "美少女" {
		t.Fatalf("tags = %+v", movie.Tags)
	}
	if len(result.Images) != 1 || result.Images[0].Role != "cover" || result.Images[0].Provider != "missav" {
		t.Fatalf("images = %+v", result.Images)
	}
}

func TestParseMissAVRejectsMismatchedCode(t *testing.T) {
	body := `<html><body>
<span class="text-secondary">識別碼:</span><span class="font-medium text-secondary"> SSIS-002 </span>
</body></html>`
	if _, err := parseMissAV(body, "SSIS-001"); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("expected not found for mismatched code, got %v", err)
	}
}

func TestMissAVRouting(t *testing.T) {
	s := NewMissAV(nil)
	defer s.Close()
	if s.ID() != "missav" {
		t.Fatalf("id = %s", s.ID())
	}
	for _, code := range []string{"SSIS-001", "abp-123", "IPX-542"} {
		if !s.Supports(code) {
			t.Fatalf("expected %s supported", code)
		}
	}
	for _, code := range []string{"FC2-123456", "FC2-PPV-1234567", "HEYZO-1234", ""} {
		if s.Supports(code) {
			t.Fatalf("expected %s unsupported", code)
		}
	}
	ref := domain.MovieRef{Code: "FC2-123456"}
	if _, err := s.Fetch(t.Context(), ref); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("unsupported code fetched: %v", err)
	}
}

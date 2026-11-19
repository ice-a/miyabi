package scan

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/storage"
)

// findNFO selects a matching NFO according to priority:
// 1. Exact filename match (e.g. STEM.nfo)
// 2. Catalogue code equivalence match (e.g. 200GANA vs GANA)
// 3. Sole NFO in an unshared/private folder
func findNFO(code string, shared bool, files []storage.File) (storage.File, bool) {
	wanted := nfo.FileStem(code) + ".nfo"
	var alias, candidate storage.File
	aliasFound, candidates := false, 0
	for _, entry := range files {
		if entry.IsDirectory {
			continue
		}
		filename := entry.Name
		if strings.EqualFold(filename, wanted) {
			return entry, true
		}
		if !strings.EqualFold(path.Ext(filename), ".nfo") {
			continue
		}
		candidates++
		candidate = entry
		if !aliasFound {
			if candidateCode, ok := codeid.Parse(filename); ok && codeid.IsEquivalent(candidateCode, code) {
				alias, aliasFound = entry, true
			}
		}
	}
	if aliasFound {
		return alias, true
	}
	if !shared && candidates == 1 {
		return candidate, true
	}
	return storage.File{}, false
}

// readNFO reads and decodes an NFO file from the mounted directory.
func readNFO(ctx context.Context, sess storage.Session, entry storage.File) (nfo.Movie, error) {
	body, err := sess.Read(ctx, entry.ID, 2<<20)
	if err != nil {
		return nfo.Movie{}, fmt.Errorf("read %s: %w", entry.Name, err)
	}
	doc, err := nfo.Decode(body)
	if err != nil {
		return nfo.Movie{}, err
	}
	code := codeid.Normalize(doc.Code)
	if code == "" {
		code, _ = codeid.Parse(entry.Name)
	}
	doc.Code = code
	return doc, nil
}

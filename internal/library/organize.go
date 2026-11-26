package library

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
)

// organizeLimit keeps the report bounded; the totals still show up in the
// summary issue and re-running the report is cheap.
const organizeLimit = 100

// OrganizeIssue is one naming suggestion, keyed by kind:
// "misnamed_file" (file name misses the movie code and the code-less folder
// around it does too). FileID identifies the indexed row so a suggestion can
// be applied through POST /api/library/organize/apply.
type OrganizeIssue struct {
	Kind string `json:"kind"`
	Code string `json:"code,omitempty"`
	Path string `json:"path,omitempty"`
	// SuggestedName is the canonical "<CODE>.<ext>" name the file could take.
	SuggestedName string `json:"suggested_name,omitempty"`
	Count         int    `json:"count,omitempty"`
	FileID        int    `json:"file_id,omitempty"`
}

// OrganizeReport is the media-organization report served by
// GET /api/library/organize. It proposes names; renaming happens only through
// the apply endpoint.
type OrganizeReport struct {
	GeneratedAt string          `json:"generated_at"`
	Issues      []OrganizeIssue `json:"issues"`
	// Undoable reports whether the rename journal holds a batch to undo.
	Undoable bool `json:"undoable"`
}

// Organize compares every indexed file name against its movie code using the
// same equivalence rules as the scanner (codeid.Candidates), so a suggestion
// here mirrors what the next scan would index. Files that already carry an
// equivalent code — or sit in a directory that does — are skipped: renaming
// one directory fixes every file inside it.
func (s *Service) Organize(ctx context.Context) (OrganizeReport, error) {
	report := OrganizeReport{GeneratedAt: nowString(), Issues: []OrganizeIssue{}}

	records, err := s.database.File.Query().
		Where(s.scope(ctx), file.HasMovie()).
		Select(file.FieldPath, file.FieldName, file.FieldMovieID).
		All(ctx)
	if err != nil {
		return report, fmt.Errorf("list library files: %w", err)
	}

	codes, err := s.movieCodes(ctx, records)
	if err != nil {
		return report, err
	}

	misnamed := 0
	for _, record := range records {
		code := ""
		if record.MovieID != nil {
			code = codes[*record.MovieID]
		}
		if code == "" {
			continue
		}
		if nameCarriesCode(record.Name, code) {
			continue
		}
		if nameCarriesCode(filepath.Base(filepath.Dir(record.Path)), code) {
			continue
		}
		suggested := canonicalName(code, record.Name)
		if suggested == "" {
			// Not a media extension we can safely rename; leave it alone.
			continue
		}
		misnamed++
		if len(report.Issues) < organizeLimit {
			report.Issues = append(report.Issues, OrganizeIssue{
				Kind:          "misnamed_file",
				Code:          code,
				Path:          record.Path,
				SuggestedName: suggested,
				FileID:        record.ID,
			})
		}
	}
	if misnamed > len(report.Issues) {
		report.Issues = append(report.Issues, OrganizeIssue{Kind: "misnamed_file_summary", Count: misnamed})
	}
	// An unreadable journal must not take down the report; the undo button
	// simply stays hidden.
	if journal, journalErr := s.loadJournal(ctx); journalErr == nil {
		report.Undoable = len(journal) > 0
	} else {
		slog.WarnContext(ctx, "failed to load organize journal", "error", journalErr)
	}
	return report, nil
}

// nameCarriesCode reports whether name contains any candidate form of code.
func nameCarriesCode(name, code string) bool {
	if code == "" || name == "" {
		return false
	}
	upper := strings.ToUpper(name)
	for _, candidate := range codeid.Candidates(code) {
		if candidate != "" && strings.Contains(upper, strings.ToUpper(candidate)) {
			return true
		}
	}
	return false
}

// canonicalName proposes "<CODE>.<ext>" for an indexed media file, keeping the
// original extension. Extensions are judged by the same rules as the scanner
// (domain.IsVideo plus .strm); anything else yields "" so callers skip the
// file instead of renaming away its extension.
func canonicalName(code, name string) string {
	if code == "" || name == "" {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".strm" && !domain.IsVideo(name) {
		return ""
	}
	return code + ext
}

func nowString() string {
	return time.Now().Format(time.RFC3339)
}

// organizeJournalSetting keeps the latest rename batches so the last one can
// be undone. Old entries fall off once the list grows past the cap.
const organizeJournalSetting = "library.organize_journal"
const organizeJournalCap = 20

// OrganizeRename reports a single rename performed by apply or undo.
type OrganizeRename struct {
	FileID int    `json:"file_id"`
	From   string `json:"from"`
	To     string `json:"to"`
	// Error explains a failed rename; empty on success.
	Error string `json:"error,omitempty"`
}

// OrganizeApplyReport is the response of the apply and undo endpoints.
type OrganizeApplyReport struct {
	Renames []OrganizeRename `json:"renames"`
	// Undoable reports whether another batch remains in the journal.
	Undoable bool `json:"undoable"`
}

type organizeRenameRecord struct {
	FileID  int    `json:"file_id"`
	Dir     string `json:"dir"`
	OldName string `json:"old_name"`
	NewName string `json:"new_name"`
}

type organizeJournalEntry struct {
	At      string                 `json:"at"`
	Renames []organizeRenameRecord `json:"renames"`
}

// ApplyOrganize renames the requested indexed files to their canonical
// "<CODE>.<ext>" names. Suggestions are recomputed from the current database
// state, so stale reports never rename a file that already carries its code.
// The batch is journalled (in the settings table) for a single-level undo.
func (s *Service) ApplyOrganize(ctx context.Context, fileIDs []int) (OrganizeApplyReport, error) {
	report := OrganizeApplyReport{Renames: []OrganizeRename{}}
	if len(fileIDs) == 0 {
		return report, domain.E(domain.KindInvalid, "请先选择要重命名的文件", nil)
	}
	if err := s.scanLock.Lock(ctx); err != nil {
		return report, err
	}
	defer s.scanLock.Unlock()

	records, err := s.database.File.Query().
		Where(s.scope(ctx), file.IDIn(fileIDs...), file.HasMovie()).
		Select(file.FieldID, file.FieldPath, file.FieldName, file.FieldMovieID).
		All(ctx)
	if err != nil {
		return report, fmt.Errorf("load files to rename: %w", err)
	}
	if len(records) == 0 {
		return report, domain.E(domain.KindNotFound, "所选文件不在当前媒体库中", nil)
	}

	codes, err := s.movieCodes(ctx, records)
	if err != nil {
		return report, err
	}

	applied := make([]organizeRenameRecord, 0, len(records))
	targets := make(map[string]bool, len(records))
	for _, record := range records {
		code := ""
		if record.MovieID != nil {
			code = codes[*record.MovieID]
		}
		if code == "" {
			continue
		}
		suggested := canonicalName(code, record.Name)
		if suggested == record.Name || suggested == "" {
			// The report was stale or the extension is unusual; nothing to do.
			continue
		}
		key := filepath.Dir(record.Path) + "\x00" + suggested
		if targets[key] {
			// Two files of the same movie would fight over one name.
			report.Renames = append(report.Renames, OrganizeRename{
				FileID: record.ID, From: record.Path,
				Error: "同目录中已有本批的重命名目标",
			})
			continue
		}
		targets[key] = true
		target := filepath.Join(filepath.Dir(record.Path), suggested)
		if _, statErr := os.Stat(target); statErr == nil {
			report.Renames = append(report.Renames, OrganizeRename{
				FileID: record.ID, From: record.Path, To: target,
				Error: "目标文件名已存在",
			})
			continue
		}
		if err := os.Rename(record.Path, target); err != nil {
			report.Renames = append(report.Renames, OrganizeRename{
				FileID: record.ID, From: record.Path, To: target,
				Error: domain.PublicMessage(fmt.Errorf("rename: %w", err)),
			})
			continue
		}
		if err := s.database.File.UpdateOneID(record.ID).
			SetPath(target).SetName(suggested).Exec(ctx); err != nil {
			// Disk is already renamed; rolling it back here would fight the
			// index. Surface the inconsistency so a rescan can reconcile.
			report.Renames = append(report.Renames, OrganizeRename{
				FileID: record.ID, From: record.Path, To: target,
				Error: "磁盘已重命名，但索引更新失败，请重新扫描",
			})
			continue
		}
		report.Renames = append(report.Renames, OrganizeRename{
			FileID: record.ID, From: record.Path, To: target,
		})
		applied = append(applied, organizeRenameRecord{
			FileID: record.ID, Dir: filepath.Dir(record.Path),
			OldName: record.Name, NewName: suggested,
		})
	}
	if len(applied) == 0 {
		return report, nil
	}

	journal, err := s.loadJournal(ctx)
	if err != nil {
		return report, err
	}
	journal = append(journal, organizeJournalEntry{At: nowString(), Renames: applied})
	if len(journal) > organizeJournalCap {
		journal = journal[len(journal)-organizeJournalCap:]
	}
	if err := database.SaveSetting(ctx, s.database, organizeJournalSetting, journal); err != nil {
		return report, err
	}
	report.Undoable = true
	s.tasks.NotifyLibraryChanged()
	return report, nil
}

// UndoOrganize reverses the most recent applied rename batch, on disk and in
// the index, and drops that journal entry.
func (s *Service) UndoOrganize(ctx context.Context) (OrganizeApplyReport, error) {
	report := OrganizeApplyReport{Renames: []OrganizeRename{}}
	if err := s.scanLock.Lock(ctx); err != nil {
		return report, err
	}
	defer s.scanLock.Unlock()

	journal, err := s.loadJournal(ctx)
	if err != nil {
		return report, err
	}
	if len(journal) == 0 {
		return report, domain.E(domain.KindInvalid, "没有可撤销的重命名记录", nil)
	}
	entry := journal[len(journal)-1]

	// Failures stay in the journal so the next undo can retry them; only a
	// fully undone batch leaves the journal.
	var failed []organizeRenameRecord
	for _, record := range entry.Renames {
		from := filepath.Join(record.Dir, record.NewName)
		to := filepath.Join(record.Dir, record.OldName)
		if _, statErr := os.Stat(to); statErr == nil {
			report.Renames = append(report.Renames, OrganizeRename{
				FileID: record.FileID, From: from, To: to,
				Error: "原文件名已存在",
			})
			failed = append(failed, record)
			continue
		}
		if err := os.Rename(from, to); err != nil {
			report.Renames = append(report.Renames, OrganizeRename{
				FileID: record.FileID, From: from, To: to,
				Error: domain.PublicMessage(fmt.Errorf("rename: %w", err)),
			})
			failed = append(failed, record)
			continue
		}
		if err := s.database.File.UpdateOneID(record.FileID).
			SetPath(to).SetName(record.OldName).Exec(ctx); err != nil {
			report.Renames = append(report.Renames, OrganizeRename{
				FileID: record.FileID, From: from, To: to,
				Error: "磁盘已重命名，但索引更新失败，请重新扫描",
			})
			failed = append(failed, record)
			continue
		}
		report.Renames = append(report.Renames, OrganizeRename{
			FileID: record.FileID, From: from, To: to,
		})
	}

	if len(failed) > 0 {
		journal[len(journal)-1] = organizeJournalEntry{At: entry.At, Renames: failed}
	} else {
		journal = journal[:len(journal)-1]
	}
	if err := database.SaveSetting(ctx, s.database, organizeJournalSetting, journal); err != nil {
		return report, err
	}
	report.Undoable = len(journal) > 0
	s.tasks.NotifyLibraryChanged()
	return report, nil
}

// movieCodes loads the codes for the movies behind the given file records.
func (s *Service) movieCodes(ctx context.Context, records []*ent.File) (map[int]string, error) {
	movieIDs := make([]int, 0, len(records))
	seen := make(map[int]bool, len(records))
	for _, record := range records {
		if record.MovieID != nil && !seen[*record.MovieID] {
			seen[*record.MovieID] = true
			movieIDs = append(movieIDs, *record.MovieID)
		}
	}
	codes := make(map[int]string, len(movieIDs))
	if len(movieIDs) == 0 {
		return codes, nil
	}
	rows, err := s.database.Movie.Query().
		Where(movie.IDIn(movieIDs...)).
		Select(movie.FieldID, movie.FieldCode).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load movie codes: %w", err)
	}
	for _, row := range rows {
		codes[row.ID] = row.Code
	}
	return codes, nil
}

func (s *Service) loadJournal(ctx context.Context) ([]organizeJournalEntry, error) {
	journal, _, err := database.LoadSetting[[]organizeJournalEntry](ctx, s.database, organizeJournalSetting)
	if err != nil {
		return nil, fmt.Errorf("load organize journal: %w", err)
	}
	return journal, nil
}

package storage

import (
	"context"
)

// WalkPages reads every page of one directory, verifying that the listing did
// not change underneath the walk.
func WalkPages(ctx context.Context, fetch func(offset int) (FilePage, error), visit func(FilePage) (bool, error)) error {
	total, offset := -1, 0
	for {
		page, err := fetch(offset)
		if err != nil {
			return err
		}
		if total < 0 {
			total = page.Total
		}
		if page.Total != total || total < 0 || offset > total ||
			(page.HasMore && len(page.Files) == 0) ||
			(!page.HasMore && offset+len(page.Files) != total) {
			return ErrDirectoryIncomplete
		}
		if done, err := visit(page); err != nil || done {
			return err
		}
		if !page.HasMore {
			return nil
		}
		offset += len(page.Files)
	}
}

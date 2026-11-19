package scan

import "github.com/ppxb/miyabi/internal/storage"

// Directory describes a directory queued during BFS traversal.
type Directory struct {
	ID   string
	Path string
}

// Video wraps an indexed file with an assigned catalogue code.
type Video struct {
	storage.File
	Code   string
	Manual bool
	// DirectoryCode records a provisional part group for transactional validation.
	DirectoryCode string
}

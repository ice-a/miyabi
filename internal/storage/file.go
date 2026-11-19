package storage

import (
	"crypto/sha1"
	"encoding/hex"
	"path"
)

// Directory names one path segment of a storage location.
type Directory struct {
	ID   string
	Name string
}

// File describes one directory entry listed by a storage provider.
//
// Local entries identify themselves through Path; remote providers also set
// PickCode so downloads can be resolved without listing the parent again.
type File struct {
	ID          string
	ParentID    string
	Name        string
	IsDirectory bool
	Size        int64
	PickCode    string
	SHA1        string
	// UploadTime is the provider's own upload timestamp in Unix seconds; zero
	// when the provider does not expose one.
	UploadTime int64
	// Path is the absolute filesystem path of local entries.
	Path string
}

// FilePage is one slice of a directory listing plus the directory's own path.
type FilePage struct {
	Files   []File
	Path    []Directory
	Total   int
	HasMore bool
}

// SHA1 returns the upper-case hex SHA1 digest of body, matching the digest form
// stored for indexed files.
func SHA1(body []byte) string {
	sum := sha1.Sum(body)
	return hex.EncodeToString(sum[:])
}

// FilePath joins a directory path and a file name into a root-anchored path.
func FilePath(segments []Directory, name string) string {
	return path.Join(directoryPath(segments), name)
}

// directoryPath builds a root-anchored path string from listed segments.
func directoryPath(segments []Directory) string {
	names := make([]string, 0, len(segments))
	for _, directory := range segments {
		if directory.Name != "" {
			names = append(names, directory.Name)
		}
	}
	return "/" + path.Join(names...)
}

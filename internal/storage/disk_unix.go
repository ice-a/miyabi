//go:build !windows

package storage

import "syscall"

// DiskUsage reports the capacity of the volume holding a directory.
type DiskUsage struct {
	TotalBytes uint64 `json:"total_bytes"`
	FreeBytes  uint64 `json:"free_bytes"`
	UsedBytes  uint64 `json:"used_bytes"`
}

// diskUsage queries the volume statistics for path (POSIX statfs).
func diskUsage(path string) (DiskUsage, error) {
	var stats syscall.Statfs_t
	if err := syscall.Statfs(path, &stats); err != nil {
		return DiskUsage{}, err
	}
	total := uint64(stats.Blocks) * uint64(stats.Bsize)
	free := uint64(stats.Bavail) * uint64(stats.Bsize)
	return DiskUsage{TotalBytes: total, FreeBytes: free, UsedBytes: total - free}, nil
}

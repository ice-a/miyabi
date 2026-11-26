//go:build windows

package storage

import "golang.org/x/sys/windows"

// DiskUsage reports the capacity of the volume holding a directory.
type DiskUsage struct {
	TotalBytes uint64 `json:"total_bytes"`
	FreeBytes  uint64 `json:"free_bytes"`
	UsedBytes  uint64 `json:"used_bytes"`
}

// diskUsage queries the volume statistics for path (Windows).
func diskUsage(path string) (DiskUsage, error) {
	var free, total, available uint64
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return DiskUsage{}, err
	}
	if err := windows.GetDiskFreeSpaceEx(pathPtr, &available, &total, &free); err != nil {
		return DiskUsage{}, err
	}
	return DiskUsage{TotalBytes: total, FreeBytes: free, UsedBytes: total - free}, nil
}

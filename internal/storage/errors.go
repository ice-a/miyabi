package storage

import (
	"io/fs"

	"github.com/ppxb/miyabi/internal/domain"
)

// ErrSourceChanged is returned when an indexed file no longer belongs to the
// source recorded in the library index.
var ErrSourceChanged = domain.E(domain.KindConflict, "媒体目录已变更，请重新扫描", nil)

// ErrDirectoryIncomplete is returned when a directory listing is inconsistent
// across pages, which means the tree changed while it was being read.
var ErrDirectoryIncomplete = domain.E(domain.KindConflict, "目录内容在读取期间变化或分页不完整，请重新扫描", nil)

// ErrNotStreamable is returned when a local file cannot be opened for playback,
// for example because it was removed or moved after the scan.
var ErrNotStreamable = domain.E(domain.KindNotFound, "视频文件已移动或删除，请重新扫描", nil)

// ErrFolderMoved is returned when a directory is listed outside the mounted root.
var ErrFolderMoved = domain.E(domain.KindConflict, "该文件夹已移出媒体目录，请重新扫描", nil)

// ErrVideoGone is returned when an indexed file is no longer present.
var ErrVideoGone = domain.E(domain.KindNotFound, "下载资源已移出媒体目录", fs.ErrNotExist)

// ErrVideoGoneMessage is the public text used when a video disappeared.
const ErrVideoGoneMessage = "下载资源已移出媒体目录"

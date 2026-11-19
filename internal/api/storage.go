package api

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
)

type StorageManager interface {
	Source() *domain.LibrarySource
	Mount(context.Context, domain.LibraryDirectory) (domain.LibrarySource, error)
	Clear(context.Context) error
}

// storageDirectory describes the directory the library currently reads from.
type storageDirectory struct {
	Mounted   bool                    `json:"mounted"`
	Directory domain.LibraryDirectory `json:"directory"`
}

func storageDirectoryHandler(service StorageManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		result := storageDirectory{}
		if source := service.Source(); source != nil {
			result.Mounted = true
			result.Directory = source.Directory
		}
		respond(c, result, nil)
	}
}

func storageMountHandler(service StorageManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, ok := bindJSON[storageMountInput](c)
		if !ok {
			return
		}
		source, err := service.Mount(c.Request.Context(), domain.LibraryDirectory{ID: body.Path, Name: body.Name})
		if err != nil {
			c.Error(err)
			return
		}
		respond(c, storageDirectory{Mounted: true, Directory: source.Directory}, nil)
	}
}

func storageClearHandler(service StorageManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := service.Clear(c.Request.Context()); err != nil {
			c.Error(err)
			return
		}
		respond(c, storageDirectory{}, nil)
	}
}

type storageMountInput struct {
	Path string `json:"path" binding:"required"`
	Name string `json:"name"`
}

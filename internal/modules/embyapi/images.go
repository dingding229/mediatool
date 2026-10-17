package embyapi

import (
	"context"
	"errors"
)

type ImageRequest struct {
	ItemID         string
	Type           string
	Index          int
	RequestedWidth int
	Tag            string
	Format         string
}
type ImageResult struct {
	Body             []byte
	ContentType, Tag string
	MaxAge           int
}
type ImageResolver interface {
	ResolveImage(context.Context, ImageRequest) (ImageResult, error)
}

var (
	ErrImageNotFound    = errors.New("native image not found")
	ErrImageUnavailable = errors.New("native image unavailable")
)

package clouddrive2

import (
	"context"
	"errors"
	"io"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/storage"
	"time"
)

// Iterate streams a directory instead of treating the UI List cap as a full
// listing. No local indexes are deleted when a stream fails or is truncated.
func (a *Adapter) Iterate(ctx context.Context, directory string, visit func(storage.File) error) error {
	ctx, releaseOperation, acquireErr := a.acquire(ctx)
	if acquireErr != nil {
		return acquireErr
	}
	defer releaseOperation()
	bounded, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	token, err := a.tokens.get(bounded)
	if err != nil {
		return sanitizeError(err)
	}
	stream, err := a.client.GetSubFiles(withToken(bounded, token), &pb.ListSubFileRequest{Path: directory, ForceRefresh: true})
	if err != nil {
		return sanitizeError(err)
	}
	count := 0
	for {
		reply, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return sanitizeError(err)
		}
		for _, raw := range reply.GetSubFiles() {
			if raw == nil {
				continue
			}
			count++
			if count > 100000 {
				return storage.ErrLimitExceeded
			}
			if err = visit(mapFile(raw)); err != nil {
				return err
			}
		}
	}
}

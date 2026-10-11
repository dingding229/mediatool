package clouddrive2

import (
	"context"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/storage"
	"path"
)

// No mutating RPC is automatically replayed on an authentication error: its
// commit may be unknown. The caller journals and reconciles observed identity.
func (a *Adapter) fileOperation(ctx context.Context, call func(context.Context) (*pb.FileOperationResult, error)) error {
	ctx, release, err := a.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	bounded, cancel := context.WithTimeout(ctx, a.options.RequestTimeout)
	defer cancel()
	token, err := a.tokens.get(bounded)
	if err != nil {
		return sanitizeError(err)
	}
	result, err := call(withToken(bounded, token))
	if err != nil {
		return sanitizeError(err)
	}
	if result == nil || !result.GetSuccess() {
		return storage.ErrInvalidResponse
	}
	return nil
}
func (a *Adapter) RenamePath(ctx context.Context, source, name string) error {
	if source == "/" || name == "" || path.Base(name) != name || name == "." || name == ".." {
		return storage.ErrInvalidResponse
	}
	return a.fileOperation(ctx, func(ctx context.Context) (*pb.FileOperationResult, error) {
		return a.client.RenameFile(ctx, &pb.RenameFileRequest{TheFilePath: source, NewName: name})
	})
}
func (a *Adapter) MovePath(ctx context.Context, source, parent string) error {
	policy := pb.MoveFileRequest_Skip
	across := false
	recursive := false
	return a.fileOperation(ctx, func(ctx context.Context) (*pb.FileOperationResult, error) {
		return a.client.MoveFile(ctx, &pb.MoveFileRequest{TheFilePaths: []string{source}, DestPath: parent, ConflictPolicy: &policy, MoveAcrossClouds: &across, HandleConflictRecursively: &recursive})
	})
}
func (a *Adapter) MakeDirectory(ctx context.Context, parent, name string) error {
	return a.fileOperation(ctx, func(ctx context.Context) (*pb.FileOperationResult, error) {
		reply, err := a.client.CreateFolder(ctx, &pb.CreateFolderRequest{ParentPath: parent, FolderName: name})
		if reply == nil {
			return nil, err
		}
		return reply.GetResult(), err
	})
}

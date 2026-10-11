package clouddrive2

import (
	"context"
	"errors"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/storage"
)

func (a *Adapter) ControlUpload(ctx context.Context, id, action string) error {
	ctx, releaseOperation, acquireErr := a.acquire(ctx)
	if acquireErr != nil {
		return acquireErr
	}
	defer releaseOperation()
	if id == "" {
		return errors.New("upload session unknown")
	}
	request := &pb.RemoteUploadControlRequest{UploadId: id}
	switch action {
	case "pause":
		request.Control = &pb.RemoteUploadControlRequest_Pause{Pause: &pb.PauseRemoteUpload{}}
	case "resume":
		request.Control = &pb.RemoteUploadControlRequest_Resume{Resume: &pb.ResumeRemoteUpload{}}
	case "cancel":
		request.Control = &pb.RemoteUploadControlRequest_Cancel{Cancel: &pb.CancelRemoteUpload{}}
	default:
		return errors.New("unsupported upload control")
	}
	bounded, cancel := context.WithTimeout(ctx, a.options.RequestTimeout)
	defer cancel()
	_, err := authorized(bounded, a, func(c context.Context) (any, error) { return a.client.RemoteUploadControl(c, request) })
	return sanitizeUploadError(err)
}

type UploadSession struct {
	ID    string `json:"id"`
	Size  uint64 `json:"size"`
	State string `json:"state"`
}

func (a *Adapter) UploadsAt(ctx context.Context, destination string) ([]UploadSession, error) {
	ctx, releaseOperation, acquireErr := a.acquire(ctx)
	if acquireErr != nil {
		return nil, acquireErr
	}
	defer releaseOperation()
	out := []UploadSession{}
	for page := uint32(1); page <= 100; page++ {
		bounded, cancel := context.WithTimeout(ctx, a.options.RequestTimeout)
		reply, err := authorized(bounded, a, func(c context.Context) (*pb.GetUploadFileListResult, error) {
			return a.client.GetUploadFileList(c, &pb.GetUploadFileListRequest{ItemsPerPage: 200, PageNumber: page})
		})
		cancel()
		if err != nil {
			return nil, sanitizeError(err)
		}
		for _, item := range reply.GetUploadFiles() {
			if item.GetDestPath() != destination {
				continue
			}
			switch item.GetStatusEnum() {
			case pb.UploadFileInfo_Finish, pb.UploadFileInfo_Skipped, pb.UploadFileInfo_Cancelled, pb.UploadFileInfo_Ignored:
				continue
			}
			out = append(out, UploadSession{ID: item.GetKey(), Size: item.GetSize(), State: item.GetStatusEnum().String()})
		}
		total := reply.GetTotalCount()
		if reply.GetTotalCountFiltered() > 0 {
			total = reply.GetTotalCountFiltered()
		}
		if page*200 >= total {
			return out, nil
		}
		if len(reply.GetUploadFiles()) == 0 {
			return nil, storage.ErrInvalidResponse
		}
	}
	return nil, storage.ErrLimitExceeded
}
func (a *Adapter) ActiveUploadAt(ctx context.Context, destination string) (bool, error) {
	list, err := a.UploadsAt(ctx, destination)
	return len(list) > 0, err
}

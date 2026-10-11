package clouddrive2

import (
	"context"
	"errors"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/storage"
)

// TryDownloadUserAgent changes only the documented customUserAgent preference.
// The explicit maintenance caller must verify the resulting direct link. Failed
// verification restores only this field after rereading all current settings.
// Permission denial is never bypassed. The preference is not an auth credential.
func (a *Adapter) TryDownloadUserAgent(ctx context.Context, cloudPath, agent string, verify func(context.Context) error) (bool, error) {
	if agent == "" || len(agent) > 512 || strings.ContainsAny(agent, "\r\n\x00") || verify == nil {
		return false, storage.ErrInvalidResponse
	}
	ctx, release, acquireErr := a.acquire(ctx)
	if acquireErr != nil {
		return false, acquireErr
	}
	defer release()
	ctx, stop := context.WithTimeout(ctx, min(a.options.RequestTimeout*4, 40*time.Second))
	defer stop()
	file, err := authorized(ctx, a, func(c context.Context) (*pb.CloudDriveFile, error) {
		return a.client.FindFileByPath(c, &pb.FindFileByPathRequest{Path: cloudPath})
	})
	if err != nil {
		return false, sanitizeError(err)
	}
	if file == nil || file.GetCloudAPI() == nil {
		return false, storage.ErrInvalidResponse
	}
	api := file.GetCloudAPI()
	get := func(c context.Context) (*pb.CloudAPIConfig, error) {
		return authorized(c, a, func(auth context.Context) (*pb.CloudAPIConfig, error) {
			return a.client.GetCloudAPIConfig(auth, &pb.GetCloudAPIConfigRequest{CloudName: api.GetName(), UserName: api.GetUserName()})
		})
	}
	set := func(c context.Context, cfg *pb.CloudAPIConfig) error {
		_, err := authorized(c, a, func(auth context.Context) (*emptypb.Empty, error) {
			return a.client.SetCloudAPIConfig(auth, &pb.SetCloudAPIConfigRequest{CloudName: api.GetName(), UserName: api.GetUserName(), Config: cfg})
		})
		if err != nil {
			return sanitizeError(err)
		}
		return nil
	}
	current, err := get(ctx)
	if err != nil {
		return false, sanitizeError(err)
	}
	if current == nil {
		return false, storage.ErrInvalidResponse
	}
	if current.GetCustomUserAgent() == agent {
		return false, verify(ctx)
	}
	old := current.CustomUserAgent
	next := proto.Clone(current).(*pb.CloudAPIConfig)
	next.CustomUserAgent = proto.String(agent)
	restore := func() error {
		restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		latest, restoreErr := get(restoreCtx)
		if restoreErr != nil || latest == nil {
			return storage.ErrUnavailable
		}
		sameOld := (old == nil && latest.CustomUserAgent == nil) || (old != nil && latest.CustomUserAgent != nil && *old == *latest.CustomUserAgent)
		if sameOld {
			return nil
		}
		// A concurrent unrelated UA change is not ours to overwrite.
		if latest.GetCustomUserAgent() != agent {
			return errors.New("CloudDrive user-agent concurrently changed; restoration unconfirmed")
		}
		latest.CustomUserAgent = old
		return set(restoreCtx, latest)
	}
	if err = set(ctx, next); err != nil {
		if errors.Is(err, storage.ErrForbidden) || errors.Is(err, storage.ErrUnsupported) || errors.Is(err, storage.ErrUnauthenticated) {
			return false, err
		}
		// A timeout can occur after the mutation. Read back and reconcile before
		// returning; never assume that an uncertain Set left configuration intact.
		if restoreErr := restore(); restoreErr != nil {
			return true, errors.New("CloudDrive user-agent update outcome unconfirmed")
		}
		return false, err
	}
	if err = verify(ctx); err == nil {
		return true, nil
	}
	if restoreErr := restore(); restoreErr != nil {
		return true, errors.New("CloudDrive user-agent verification failed and restoration unconfirmed")
	}
	return false, err
}

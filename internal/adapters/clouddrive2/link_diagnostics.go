package clouddrive2

import (
	"context"
	"google.golang.org/protobuf/types/known/emptypb"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/storage"
	"time"
)

// LinkDiagnostics exposes capability booleans only. It must never serialize
// signed URLs, provider account identities, cookies or raw cloud configuration.
type LinkDiagnostics struct {
	ProductVersion            string `json:"productVersion,omitempty"`
	DirectURLPresent          bool   `json:"directUrlPresent"`
	RelayPathPresent          bool   `json:"relayPathPresent"`
	CloudConfigReadable       bool   `json:"cloudConfigReadable"`
	DirectLinkEnabled         bool   `json:"directLinkEnabled"`
	ProviderSupportsDirectURL bool   `json:"providerSupportsDirectUrl"`
	HTTPDownloadEnabled       bool   `json:"httpDownloadEnabled"`
	FileIsLocal               bool   `json:"fileIsLocal"`
	FileIsCloudFile           bool   `json:"fileIsCloudFile"`
}

func (a *Adapter) DiagnoseLink(ctx context.Context, cloudPath string) (LinkDiagnostics, error) {
	ctx, release, err := a.acquire(ctx)
	if err != nil {
		return LinkDiagnostics{}, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, min(a.options.RequestTimeout*3, 30*time.Second))
	defer cancel()
	var out LinkDiagnostics
	runtime, err := authorized(ctx, a, func(c context.Context) (*pb.RuntimeInfo, error) { return a.client.GetRuntimeInfo(c, &emptypb.Empty{}) })
	if err == nil && runtime != nil {
		out.ProductVersion = runtime.GetProductVersion()
	}
	file, err := authorized(ctx, a, func(c context.Context) (*pb.CloudDriveFile, error) {
		return a.client.FindFileByPath(c, &pb.FindFileByPathRequest{Path: cloudPath})
	})
	if err != nil {
		return out, sanitizeError(err)
	}
	if file == nil {
		return out, storage.ErrInvalidResponse
	}
	out.FileIsLocal = file.GetIsLocal()
	out.FileIsCloudFile = file.GetIsCloudFile()
	if api := file.GetCloudAPI(); api != nil {
		cfg, err := authorized(ctx, a, func(c context.Context) (*pb.CloudAPIConfig, error) {
			return a.client.GetCloudAPIConfig(c, &pb.GetCloudAPIConfigRequest{CloudName: api.GetName(), UserName: api.GetUserName()})
		})
		if err == nil && cfg != nil {
			out.CloudConfigReadable = true
			out.DirectLinkEnabled = cfg.GetSupportDirectLink()
			out.ProviderSupportsDirectURL = cfg.GetSupportDirectDownloadUrl()
			out.HTTPDownloadEnabled = cfg.GetUseHttpDownload()
		}
	}
	link, err := authorized(ctx, a, func(c context.Context) (*pb.DownloadUrlPathInfo, error) {
		return a.client.GetDownloadUrlPath(c, &pb.GetDownloadUrlPathRequest{Path: cloudPath, GetDirectUrl: true})
	})
	if err != nil {
		return out, sanitizeError(err)
	}
	if link != nil {
		out.DirectURLPresent = link.GetDirectUrl() != ""
		out.RelayPathPresent = link.GetDownloadUrlPath() != ""
	}
	return out, nil
}

// RequireHTTPSDownloads disables only the provider's optional plaintext
// download preference. It preserves all other settings and requires the
// caller's existing allow_modify_cloud_apis permission; no role is bypassed.
func (a *Adapter) RequireHTTPSDownloads(ctx context.Context, cloudPath string) (bool, error) {
	ctx, release, err := a.acquire(ctx)
	if err != nil {
		return false, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, min(a.options.RequestTimeout*3, 30*time.Second))
	defer cancel()
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
	current, err := authorized(ctx, a, func(c context.Context) (*pb.CloudAPIConfig, error) {
		return a.client.GetCloudAPIConfig(c, &pb.GetCloudAPIConfigRequest{CloudName: api.GetName(), UserName: api.GetUserName()})
	})
	if err != nil {
		return false, sanitizeError(err)
	}
	if current == nil {
		return false, storage.ErrInvalidResponse
	}
	if !current.GetUseHttpDownload() {
		return false, nil
	}
	disabled := false
	current.UseHttpDownload = &disabled
	_, err = authorized(ctx, a, func(c context.Context) (*emptypb.Empty, error) {
		return a.client.SetCloudAPIConfig(c, &pb.SetCloudAPIConfigRequest{CloudName: api.GetName(), UserName: api.GetUserName(), Config: current})
	})
	if err != nil {
		return false, sanitizeError(err)
	}
	return true, nil
}

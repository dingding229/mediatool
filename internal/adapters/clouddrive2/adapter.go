// Package clouddrive2 uses the pinned official CloudDriveFileSrv protocol.
// All 115 file operations go through this boundary, never a private 115 API.
package clouddrive2

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/storage"
)

type Options struct {
	Endpoint, Token, Username, Password, CAFile string
	RequestTimeout                              time.Duration
	MaxListFiles                                int
}
type Adapter struct {
	lifecycle sync.Mutex
	active    int
	retired   bool
	conn      *grpc.ClientConn
	client    pb.CloudDriveFileSrvClient
	options   Options
	tokens    tokenManager
}

var _ storage.StorageProvider = (*Adapter)(nil)

// ParseEndpoint accepts a native gRPC http(s) origin or bare host:port.
// It rejects URL credentials, paths, resolver schemes and TLS-verification bypass.
func ParseEndpoint(raw string) (address, serverName string, tlsEnabled bool, err error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || strings.ContainsAny(u.Host, "\r\n\x00") {
		return "", "", false, errors.New("invalid CloudDrive2 gRPC endpoint; use http(s)://host:port")
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	n, e := strconv.Atoi(port)
	if e != nil || n < 1 || n > 65535 {
		return "", "", false, errors.New("invalid CloudDrive2 endpoint port")
	}
	return net.JoinHostPort(u.Hostname(), port), u.Hostname(), u.Scheme == "https", nil
}
func New(options Options) (*Adapter, error) {
	address, name, useTLS, err := ParseEndpoint(options.Endpoint)
	if err != nil {
		return nil, err
	}
	options.Token = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(options.Token), "Bearer "))
	if strings.ContainsAny(options.Token, "\r\n\x00") {
		return nil, errors.New("invalid CloudDrive2 token")
	}
	if options.Token == "" && (options.Username == "" || options.Password == "") {
		return nil, errors.New("configure CLOUDDRIVE_TOKEN or CLOUDDRIVE_USERNAME/CLOUDDRIVE_PASSWORD")
	}
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = 10 * time.Second
	}
	if options.MaxListFiles <= 0 {
		options.MaxListFiles = 1000
	}
	var transport credentials.TransportCredentials
	if useTLS {
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: name}
		if options.CAFile != "" {
			roots, err := x509.SystemCertPool()
			if err != nil {
				roots = x509.NewCertPool()
			}
			pem, err := os.ReadFile(options.CAFile)
			if err != nil || !roots.AppendCertsFromPEM(pem) {
				return nil, errors.New("CloudDrive2 CA file cannot be loaded")
			}
			tlsConfig.RootCAs = roots
		}
		transport = credentials.NewTLS(tlsConfig)
	} else {
		if options.CAFile != "" {
			return nil, errors.New("CloudDrive2 CA file requires an https endpoint")
		}
		transport = insecure.NewCredentials() // explicit HTTP, for a trusted private network only
	}
	conn, err := grpc.NewClient("passthrough:///"+address, grpc.WithTransportCredentials(transport), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(4<<20)))
	if err != nil {
		return nil, errors.New("CloudDrive2 gRPC client creation failed")
	}
	a := &Adapter{conn: conn, client: pb.NewCloudDriveFileSrvClient(conn), options: options}
	a.tokens = tokenManager{client: a.client, options: options}
	return a, nil
}
func (a *Adapter) Close() error { return a.conn.Close() }

func (a *Adapter) Find(ctx context.Context, cloudPath string) (storage.File, error) {
	ctx, releaseOperation, acquireErr := a.acquire(ctx)
	if acquireErr != nil {
		return storage.File{}, acquireErr
	}
	defer releaseOperation()
	ctx, cancel := context.WithTimeout(ctx, a.options.RequestTimeout)
	defer cancel()
	response, err := authorized(ctx, a, func(c context.Context) (*pb.CloudDriveFile, error) {
		return a.client.FindFileByPath(c, &pb.FindFileByPathRequest{Path: cloudPath})
	})
	if err != nil {
		return storage.File{}, sanitizeError(err)
	}
	if response == nil {
		return storage.File{}, storage.ErrInvalidResponse
	}
	file := mapFile(response)
	if file.Path == "" {
		file.Path = cloudPath
	}
	if file.Name == "" || file.Size < 0 {
		return storage.File{}, storage.ErrInvalidResponse
	}
	if response.GetIsForbidden() {
		return storage.File{}, storage.ErrForbidden
	}
	return file, nil
}
func (a *Adapter) Resolve(ctx context.Context, ref storage.FileRef) (storage.DownloadLink, error) {
	ctx, releaseOperation, acquireErr := a.acquire(ctx)
	if acquireErr != nil {
		return storage.DownloadLink{}, acquireErr
	}
	defer releaseOperation()
	ctx, cancel := context.WithTimeout(ctx, a.options.RequestTimeout)
	defer cancel()
	file, err := a.Find(ctx, ref.Path)
	if err != nil {
		return storage.DownloadLink{}, err
	}
	if file.IsDirectory {
		return storage.DownloadLink{}, storage.ErrDirectory
	}
	response, err := authorized(ctx, a, func(c context.Context) (*pb.DownloadUrlPathInfo, error) {
		return a.client.GetDownloadUrlPath(c, &pb.GetDownloadUrlPathRequest{Path: ref.Path, GetDirectUrl: true, Preview: false, LazyRead: false})
	})
	if err != nil {
		return storage.DownloadLink{}, sanitizeError(err)
	}
	if response == nil {
		return storage.DownloadLink{}, storage.ErrInvalidResponse
	}
	direct := strings.TrimSpace(response.GetDirectUrl())
	if direct == "" {
		return storage.DownloadLink{}, storage.ErrNoDirectURL
	}
	u, err := url.Parse(direct)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || strings.ContainsAny(direct, "\r\n\x00") {
		return storage.DownloadLink{}, storage.ErrInvalidResponse
	}
	link := storage.DownloadLink{URL: direct, UserAgent: response.GetUserAgent(), Headers: make(map[string]string, len(response.GetAdditionalHeaders()))}
	if strings.ContainsAny(link.UserAgent, "\r\n\x00") {
		return storage.DownloadLink{}, storage.ErrInvalidResponse
	}
	for k, v := range response.GetAdditionalHeaders() {
		if strings.ContainsAny(k+v, "\r\n\x00") {
			return storage.DownloadLink{}, storage.ErrInvalidResponse
		}
		link.Headers[k] = v
	}
	// Unbounded provider lifetimes are not converted into an overflowing Duration.
	if response.ExpiresIn != nil {
		seconds := min(response.GetExpiresIn(), uint64((7*24*time.Hour)/time.Second))
		link.ExpiresAt = time.Now().Add(time.Duration(seconds) * time.Second)
	}
	return link, nil
}
func (a *Adapter) List(ctx context.Context, cloudPath string) ([]storage.File, error) {
	ctx, releaseOperation, acquireErr := a.acquire(ctx)
	if acquireErr != nil {
		return nil, acquireErr
	}
	defer releaseOperation()
	ctx, cancel := context.WithTimeout(ctx, a.options.RequestTimeout)
	defer cancel()
	// A server stream that has already produced data is not replayed implicitly.
	// Unauthenticated setup/first read can be retried once before any data escapes.
	for attempt := 0; attempt < 2; attempt++ {
		token, err := a.tokens.get(ctx)
		if err != nil {
			return nil, sanitizeError(err)
		}
		stream, err := a.client.GetSubFiles(withToken(ctx, token), &pb.ListSubFileRequest{Path: cloudPath, ForceRefresh: false})
		if err != nil {
			if attempt == 0 && status.Code(err) == codes.Unauthenticated && a.tokens.invalidate(token) {
				continue
			}
			return nil, sanitizeError(err)
		}
		files := []storage.File{}
		retry := false
		for {
			reply, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return files, nil
			}
			if err != nil {
				if len(files) == 0 && attempt == 0 && status.Code(err) == codes.Unauthenticated && a.tokens.invalidate(token) {
					retry = true
					break
				}
				return nil, sanitizeError(err)
			}
			if len(files)+len(reply.GetSubFiles()) > a.options.MaxListFiles {
				return nil, storage.ErrLimitExceeded
			}
			for _, file := range reply.GetSubFiles() {
				if file != nil {
					files = append(files, mapFile(file))
				}
			}
		}
		if !retry {
			break
		}
	}
	return nil, storage.ErrUnauthenticated
}
func (a *Adapter) Probe(ctx context.Context, cloudPath string) (storage.ProbeResult, error) {
	ctx, releaseOperation, acquireErr := a.acquire(ctx)
	if acquireErr != nil {
		return storage.ProbeResult{}, acquireErr
	}
	defer releaseOperation()
	ctx, cancel := context.WithTimeout(ctx, a.options.RequestTimeout)
	defer cancel()
	response, err := a.client.GetSystemInfo(ctx, &emptypb.Empty{})
	if err != nil {
		return storage.ProbeResult{}, sanitizeError(err)
	}
	if response == nil {
		return storage.ProbeResult{}, storage.ErrInvalidResponse
	}
	result := storage.ProbeResult{LoggedIn: response.GetIsLogin(), SystemReady: response.GetSystemReady()}
	if !result.LoggedIn || !result.SystemReady {
		return result, storage.ErrNotReady
	}
	file, err := a.Find(ctx, cloudPath)
	if err != nil {
		return result, err
	}
	result.PathReadable = true
	result.File = file
	return result, nil
}

// Upload is intentionally not an implicit filesystem API. The acquisition
// worker opens a confined local file and uses UploadFile with durable session
// callbacks. Callers without session persistence must not start an upload.
func (a *Adapter) Upload(context.Context, string, string) (string, error) {
	return "", errors.New("use UploadFile with a confined source and durable upload session")
}
func mapFile(f *pb.CloudDriveFile) storage.File {
	return storage.File{ID: f.GetId(), Name: f.GetName(), Path: f.GetFullPathName(), Hashes: f.GetFileHashes(), Size: f.GetSize(), IsDirectory: f.GetIsDirectory() || f.GetFileType() == pb.CloudDriveFile_Directory}
}

func authorized[T any](ctx context.Context, a *Adapter, call func(context.Context) (T, error)) (T, error) {
	var zero T
	for attempt := 0; attempt < 2; attempt++ {
		token, err := a.tokens.get(ctx)
		if err != nil {
			return zero, err
		}
		response, err := call(withToken(ctx, token))
		if err != nil && attempt == 0 && status.Code(err) == codes.Unauthenticated && a.tokens.invalidate(token) {
			continue
		}
		return response, err
	}
	return zero, storage.ErrUnauthenticated
}
func withToken(ctx context.Context, token string) context.Context {
	return metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token))
}
func sanitizeError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return storage.ErrUnavailable
	}
	for _, known := range []error{storage.ErrUnauthenticated, storage.ErrNotReady, storage.ErrInvalidResponse} {
		if errors.Is(err, known) {
			return known
		}
	}
	switch status.Code(err) {
	case codes.NotFound:
		return storage.ErrNotFound
	case codes.Unauthenticated:
		return storage.ErrUnauthenticated
	case codes.PermissionDenied:
		return storage.ErrForbidden
	case codes.Unimplemented:
		return storage.ErrUnsupported
	case codes.DeadlineExceeded, codes.Unavailable, codes.Canceled, codes.ResourceExhausted:
		return storage.ErrUnavailable
	default:
		return storage.ErrInvalidResponse
	}
}

type tokenManager struct {
	client    pb.CloudDriveFileSrvClient
	options   Options
	mu        sync.RWMutex
	token     string
	expiresAt time.Time
	flight    singleflight.Group
}

func (t *tokenManager) get(ctx context.Context) (string, error) {
	if t.options.Token != "" {
		return t.options.Token, nil
	}
	t.mu.RLock()
	token, expires := t.token, t.expiresAt
	t.mu.RUnlock()
	if token != "" && time.Until(expires) > 30*time.Second {
		return token, nil
	}
	result := t.flight.DoChan("refresh", func() (any, error) {
		t.mu.RLock()
		token, expires := t.token, t.expiresAt
		t.mu.RUnlock()
		if token != "" && time.Until(expires) > 30*time.Second {
			return token, nil
		}
		refresh, cancel := context.WithTimeout(context.WithoutCancel(ctx), t.options.RequestTimeout)
		defer cancel()
		response, err := t.client.GetToken(refresh, &pb.GetTokenRequest{UserName: t.options.Username, Password: t.options.Password})
		if err != nil {
			return "", sanitizeError(err)
		}
		if response == nil || !response.GetSuccess() || response.GetToken() == "" {
			return "", storage.ErrUnauthenticated
		}
		expiration := response.GetExpiration()
		if expiration == nil || !expiration.IsValid() || !expiration.AsTime().After(time.Now()) {
			return "", storage.ErrInvalidResponse
		}
		if strings.ContainsAny(response.GetToken(), "\r\n\x00") {
			return "", storage.ErrInvalidResponse
		}
		t.mu.Lock()
		t.token = response.GetToken()
		t.expiresAt = expiration.AsTime()
		t.mu.Unlock()
		return response.GetToken(), nil
	})
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case value := <-result:
		if value.Err != nil {
			return "", value.Err
		}
		return value.Val.(string), nil
	}
}
func (t *tokenManager) invalidate(old string) bool {
	if t.options.Token != "" {
		return false
	} // static API tokens are not silently replaced by account login
	t.mu.Lock()
	if t.token == old {
		t.token = ""
		t.expiresAt = time.Time{}
	}
	t.mu.Unlock()
	return true
}

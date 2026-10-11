package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"media-tool/internal/adapters/clouddrive2"
	"media-tool/internal/adapters/open115"
	"media-tool/internal/adapters/qbittorrent"
	"media-tool/internal/adapters/telegram"
	"media-tool/internal/adapters/tmdb"
	"media-tool/internal/modules/sites"
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/config"
)

type cloudRevision struct {
	stamp   string
	adapter *clouddrive2.Adapter
}
type Manager struct {
	Repository   Repository
	Config       config.Config
	OutboundHTTP *http.Client
	mu           sync.Mutex
	lastPruned   time.Time
	cloud        map[string]cloudRevision
}

func NewManager(repository Repository, c config.Config) *Manager {
	return &Manager{Repository: repository, Config: c, cloud: map[string]cloudRevision{}}
}
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, client := range m.cloud {
		client.adapter.Retire()
	}
	m.cloud = map[string]cloudRevision{}
}
func (m *Manager) Cloud(ctx context.Context) (*clouddrive2.Adapter, error) {
	m.Prune(ctx)
	c, err := m.Repository.Default(ctx, "clouddrive2")
	if errors.Is(err, ErrNotFound) && m.Config.CloudDriveEndpoint != "" {
		var configured bool
		if err = m.Repository.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM integrations WHERE kind='clouddrive2')`).Scan(&configured); err != nil {
			return nil, ErrUnavailable
		}
		if !configured {
			c = Credentials{Record: Record{ID: "environment", Settings: map[string]any{"endpoint": m.Config.CloudDriveEndpoint, "caFile": m.Config.CloudDriveCAFile}}, Values: map[string]string{"token": m.Config.CloudDriveToken, "username": m.Config.CloudDriveUsername, "password": m.Config.CloudDrivePassword}}
			err = nil
		}
	}
	if err != nil {
		return nil, err
	}
	return m.cloudClient(c)
}
func (m *Manager) CloudByID(ctx context.Context, id string) (*clouddrive2.Adapter, error) {
	c, err := m.Repository.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.Record.Kind != "clouddrive2" {
		return nil, errors.New("not a CloudDrive2 connection")
	}
	return m.cloudClient(c)
}
func (m *Manager) cloudClient(c Credentials) (*clouddrive2.Adapter, error) {
	stamp := c.Record.UpdatedAt.Format(time.RFC3339Nano)
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.cloud[c.Record.ID]; ok && existing.stamp == stamp {
		return existing.adapter, nil
	}
	client, err := newCloud(c, m.Config)
	if err != nil {
		return nil, err
	}
	if old, ok := m.cloud[c.Record.ID]; ok {
		old.adapter.Retire()
	}
	m.cloud[c.Record.ID] = cloudRevision{stamp: stamp, adapter: client}
	return client, nil
}

func newCloud(c Credentials, config config.Config) (*clouddrive2.Adapter, error) {
	return clouddrive2.New(clouddrive2.Options{Endpoint: c.String("endpoint"), Token: c.Values["token"], Username: c.Values["username"], Password: c.Values["password"], CAFile: c.String("caFile"), RequestTimeout: config.CloudDriveRequestTimeout, MaxListFiles: config.CloudDriveMaxListFiles})
}
func (m *Manager) QB(ctx context.Context) (*qbittorrent.Adapter, Credentials, error) {
	c, err := m.Repository.Default(ctx, "qbittorrent")
	if err != nil {
		return nil, c, err
	}
	return &qbittorrent.Adapter{Endpoint: c.String("endpoint"), Username: c.Values["username"], Password: c.Values["password"]}, c, nil
}
func (m *Manager) TMDB(ctx context.Context) (tmdb.Client, error) {
	c, err := m.Repository.Default(ctx, "tmdb")
	if err != nil {
		return tmdb.Client{}, err
	}
	return tmdb.Client{Token: c.Values["token"], Language: c.String("language"), HTTP: m.OutboundHTTP}, nil
}
func (m *Manager) Telegram(ctx context.Context) (telegram.Client, error) {
	c, err := m.Repository.Default(ctx, "telegram")
	if err != nil {
		return telegram.Client{}, err
	}
	return telegram.Client{Token: c.Values["botToken"], ChatID: c.String("chatId"), HTTP: m.OutboundHTTP}, nil
}

type CheckResult struct {
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
	State   string `json:"state"`
}

func (m *Manager) Test(ctx context.Context, id string) (CheckResult, error) {
	c, err := m.Repository.Get(ctx, id)
	if err != nil {
		return CheckResult{}, err
	}
	result, err := m.ProbeCredentials(ctx, c)
	if err != nil {
		return result, err
	}
	if err = m.Repository.RecordCheck(ctx, id, c.Record.Revision, result.State, result.Message); err != nil {
		return CheckResult{}, ErrUnavailable
	}
	return result, nil
}

// ProbeCredentials validates prospective credentials before saving them.
func (m *Manager) ProbeCredentials(ctx context.Context, c Credentials) (CheckResult, error) {
	var err error
	bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	message := "连接检查失败，请检查设置、凭据或网络"
	state := "unavailable"
	switch c.Record.Kind {
	case "local":
		var root *os.Root
		root, err = os.OpenRoot(c.String("rootPath"))
		if err == nil {
			_, err = root.Stat(".")
			root.Close()
		}
		if err == nil {
			message = "本地目录可读取；文件操作仅限此根目录"
		}
	case "open115":
		err = m.officialClient().Probe(bounded, c.Values["accessToken"])
		if err == nil {
			message = "115官方授权有效；播放将按播放器UA取链，真实302和CDN读取仍需播放验证"
		}
		if err != nil {
			message = "115官方授权不可用或需重新扫码；请检查自己的AppID及授权"
			var api *open115.APIError
			if errors.As(err, &api) && api.RateLimited() {
				message = "115官方刷新频率限制，请稍后验证"
			}
		}
	case "clouddrive2":
		client, openErr := newCloud(c, m.Config)
		err = openErr
		if err == nil {
			defer client.Retire()
			root := c.String("cloudRoot")
			if root == "" {
				root = m.Config.STRMAllowedRoots[0]
			}
			_, err = client.Probe(bounded, root)
			if err == nil {
				message = "CloudDrive2 已就绪且凭据可读取目录；不代表真实播放器验收通过"
			}
		}
	case "qbittorrent":
		client := qbittorrent.Adapter{Endpoint: c.String("endpoint"), Username: c.Values["username"], Password: c.Values["password"]}
		var version string
		version, err = client.Probe(bounded)
		if err == nil {
			message = "qBittorrent 已连接（" + version + "）；请另行检查共享下载目录"
		}
	case "tmdb":
		err = (tmdb.Client{Token: c.Values["token"], Language: c.String("language"), HTTP: m.OutboundHTTP}).Probe(bounded)
		if err == nil {
			message = "TMDB Token 有效，可查询媒体信息"
		}
	case "telegram":
		err = (telegram.Client{Token: c.Values["botToken"], ChatID: c.String("chatId")}).Probe(bounded)
		if err == nil {
			message = "Telegram 机器人和接收目标有效；本次检查未发送通知"
		}
	case "emby":
		request, reqErr := http.NewRequestWithContext(bounded, "GET", strings.TrimRight(c.String("endpoint"), "/")+"/Users", nil)
		err = reqErr
		if err == nil {
			request.Header.Set("X-Emby-Token", c.Values["apiKey"])
			client := m.OutboundHTTP
			if client == nil {
				client = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			}
			response, requestErr := client.Do(request)
			err = requestErr
			if err == nil {
				defer response.Body.Close()
				var info []struct {
					ID string `json:"Id"`
				}
				if response.StatusCode != 200 || json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&info) != nil || len(info) == 0 {
					err = errors.New("invalid Emby response")
				} else {
					message = "Emby API Key 与用户列表可读取；真实播放需另行验收"
				}
			}
		}
	case "ptsite":
		client, siteErr := m.siteCredentials(ctx, c)
		err = siteErr
		if err == nil {
			err = client.Test(bounded)
			if err == nil {
				message = "站点登录状态已验证；真实搜索和种子下载仍需分别验证"
			}
		}
	default:
		err = errors.New("unknown integration type")
	}
	if err == nil {
		state = "ready"
	}
	if err != nil && c.Record.Kind == "clouddrive2" && errors.Is(err, storage.ErrUnauthenticated) {
		message = "CloudDrive2 凭据无效或过期"
	}
	return CheckResult{Passed: err == nil, Message: message, State: state}, nil
}

// DynamicStorage uses saved encrypted connections without restarting the server.
// Old clients stay alive until shutdown so changing a setting does not terminate
// an already-authorized stream halfway through delivery.
type DynamicStorage struct{ Manager *Manager }

func (d DynamicStorage) Resolve(ctx context.Context, ref storage.FileRef) (storage.DownloadLink, error) {
	var connectionID string
	if ref.AssetID != "" {
		started := time.Now()
		err := d.Manager.Repository.DB.QueryRow(ctx, `SELECT f.storage_id::text,f.full_path,f.size_bytes FROM media_assets a JOIN cloud_files f ON f.id=a.cloud_file_id WHERE a.id=$1 AND f.state='present'`, ref.AssetID).Scan(&connectionID, &ref.Path, &ref.ExpectedSize)
		storage.ObserveResolve(ctx, storage.StageCloudFile, started, storage.ContextFailure(ctx, err))
		if err != nil {
			if ctx.Err() != nil {
				return storage.DownloadLink{}, ctx.Err()
			}
			if errors.Is(err, pgx.ErrNoRows) {
				return storage.DownloadLink{}, storage.ErrNotFound
			}
			return storage.DownloadLink{}, storage.ErrUnavailable
		}
	}
	started := time.Now()
	official, err := d.Manager.Repository.Default(ctx, "open115")
	storage.ObserveResolve(ctx, storage.StageCloudProfile, started, storage.ContextFailure(ctx, err))
	if err == nil {
		return d.Manager.resolveOfficial(ctx, official, ref)
	}
	if !errors.Is(err, ErrNotFound) {
		return storage.DownloadLink{}, storage.ErrUnavailable
	}
	var client *clouddrive2.Adapter
	if connectionID != "" {
		client, err = d.Manager.CloudByID(ctx, connectionID)
	} else {
		client, err = d.Manager.Cloud(ctx)
	}
	if err != nil {
		return storage.DownloadLink{}, storage.ErrUnavailable
	}
	return client.Resolve(ctx, ref)
}

func (d DynamicStorage) Find(ctx context.Context, path string) (storage.File, error) {
	client, err := d.Manager.Cloud(ctx)
	if err != nil {
		return storage.File{}, storage.ErrUnavailable
	}
	return client.Find(ctx, path)
}
func (d DynamicStorage) List(ctx context.Context, path string) ([]storage.File, error) {
	client, err := d.Manager.Cloud(ctx)
	if err != nil {
		return nil, storage.ErrUnavailable
	}
	return client.List(ctx, path)
}
func (d DynamicStorage) Probe(ctx context.Context, path string) (storage.ProbeResult, error) {
	client, err := d.Manager.Cloud(ctx)
	if err != nil {
		return storage.ProbeResult{}, storage.ErrUnavailable
	}
	return client.Probe(ctx, path)
}
func (d DynamicStorage) Upload(ctx context.Context, source, dest string) (string, error) {
	client, err := d.Manager.Cloud(ctx)
	if err != nil {
		return "", storage.ErrUnavailable
	}
	return client.Upload(ctx, source, dest)
}

func (m *Manager) Site(ctx context.Context, id string) (sites.Client, error) {
	return m.siteClient(ctx, id, true)
}

// Explicit connection validation may read a disabled PT account. Every search,
// RSS and download consumer still enters Site and requires Enabled=true.
func (m *Manager) siteClient(ctx context.Context, id string, requireEnabled bool) (sites.Client, error) {
	c, err := m.Repository.Get(ctx, id)
	if err != nil {
		return sites.Client{}, err
	}
	if c.Record.Kind != "ptsite" || requireEnabled && !c.Record.Enabled {
		return sites.Client{}, errors.New("integration is not an enabled PT account")
	}
	return m.siteCredentials(ctx, c)
}
func (m *Manager) siteCredentials(ctx context.Context, c Credentials) (sites.Client, error) {
	id := c.Record.ID
	definition, err := (sites.Repository{DB: m.Repository.DB}).Get(ctx, c.String("definitionId"))
	if err != nil {
		return sites.Client{}, err
	}
	configuration, err := sites.DecodeStored(definition.RawJSON, definition.CommonJSON)
	if err != nil {
		return sites.Client{}, err
	}
	headers, err := sites.ParseAccountHeaders(c.Values["requestHeaders"])
	if err != nil {
		return sites.Client{}, ErrInvalid
	}
	return sites.Client{Config: configuration, Account: sites.Account{ID: id, Endpoint: c.String("endpoint"), UserID: c.String("userId"), Cookie: c.Values["cookie"], APIKey: c.Values["apiKey"], Passkey: c.Values["passkey"], RequestHeaders: headers}, SourceRevision: c.Record.Revision, DefinitionFingerprint: sites.DefinitionFingerprint(definition.RawJSON, definition.CommonJSON, definition.SourceCommit)}, nil
}

// Prune removes deleted/disabled profiles without terminating in-flight work.
func (m *Manager) Prune(ctx context.Context) {
	if m.Repository.DB == nil {
		return
	}
	m.mu.Lock()
	if time.Since(m.lastPruned) < 30*time.Second {
		m.mu.Unlock()
		return
	}
	m.lastPruned = time.Now()
	m.mu.Unlock()
	rows, err := m.Repository.DB.Query(ctx, `SELECT id::text,updated_at FROM integrations WHERE kind='clouddrive2' AND enabled`)
	if err != nil {
		return
	}
	current := map[string]string{}
	for rows.Next() {
		var id string
		var updated time.Time
		if rows.Scan(&id, &updated) != nil {
			rows.Close()
			return
		}
		current[id] = updated.Format(time.RFC3339Nano)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, client := range m.cloud {
		if id != "environment" && current[id] != client.stamp {
			client.adapter.Retire()
			delete(m.cloud, id)
		}
	}
}

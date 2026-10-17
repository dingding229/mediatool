// gateway-session-smoke is an administrative maintenance tool, not an API.
// Credentials arrive on stdin from a trusted Emby authentication database read,
// never arguments/logs. Explicit binding is only for pre-existing sessions whose
// token/user association has been independently established by the administrator.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"media-tool/internal/modules/emby"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/playback"
	"media-tool/internal/modules/playbackgrants"
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/cache"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

type session struct{ Token, UserID string }
type input struct {
	Sessions         []session
	ItemID, SourceID string
	ClientAgents     []string
}

func main() {
	bind := flag.Bool("bind-existing-sessions", false, "explicitly import administrator-verified existing session identities")
	export := flag.Bool("ephemeral-client-link", false, "emit ephemeral CDN capability to a private in-memory client-check pipe; never log this mode")
	flag.Parse()
	if err := run(*bind, *export); err != nil {
		fmt.Fprintln(os.Stderr, "Gateway user-session diagnostic failed; credentials, signed URLs and raw exceptions omitted.")
		os.Exit(1)
	}
}
func run(bind, export bool) error {
	var in input
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 64<<10)).Decode(&in); err != nil {
		return err
	}
	idPattern := regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	if len(in.Sessions) == 0 || len(in.Sessions) > 100 || !idPattern.MatchString(in.ItemID) || !idPattern.MatchString(in.SourceID) {
		return errors.New("invalid verified session input")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil || db == nil {
		return errors.New("database required")
	}
	defer db.Close()
	redis, err := cache.Open(cfg.RedisURL)
	if err != nil || redis == nil {
		return errors.New("redis required")
	}
	defer deferutil.Close(redis, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
	vault, err := secrets.New(cfg.SecretKey)
	if err != nil {
		return err
	}
	manager := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, cfg)
	defer manager.Close()
	connection, err := manager.Repository.Default(ctx, "emby")
	if err != nil {
		return err
	}
	upstream, err := url.Parse(connection.String("endpoint"))
	if err != nil {
		return err
	}
	store := &emby.RedisIdentityStore{Redis: redis}
	noRedirect := http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request := func(method, target, token, agent string) (*http.Response, error) {
		r, e := http.NewRequestWithContext(ctx, method, target, nil)
		if e != nil {
			return nil, e
		}
		if token != "" {
			r.Header.Set("X-Emby-Token", token)
		}
		r.Header.Set("User-Agent", agent)
		r.Header.Set("Range", "bytes=0-1023")
		return noRedirect.Do(r)
	}
	imported := 0
	if bind {
		for _, s := range in.Sessions {
			if s.Token == "" || !idPattern.MatchString(s.UserID) {
				return errors.New("invalid verified identity")
			}
			// Verify the current token remains accepted. This is NOT how the
			// association is inferred: that comes from the administrator's DB read.
			res, e := request("GET", strings.TrimRight(upstream.String(), "/")+"/Users/"+s.UserID, s.Token, "MediaTool-GatewayCheck")
			if e != nil {
				return e
			}
			var user struct {
				ID string `json:"Id"`
			}
			e = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&user)
			deferutil.Close(res.Body, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
			if e != nil || res.StatusCode != 200 || user.ID != s.UserID {
				return errors.New("verified session no longer valid")
			}
			if e = store.Remember(ctx, emby.IdentityKey(upstream, s.Token), s.UserID); e != nil {
				return e
			}
			imported++
		}
	}
	// Resolve exactly the user-authorized source under test, not the newest
	// unrelated library asset. Neither the path nor grant leaves this process.
	itemResponse, err := request("GET", strings.TrimRight(upstream.String(), "/")+"/Users/"+in.Sessions[0].UserID+"/Items/"+in.ItemID+"?Fields=MediaSources,Path", in.Sessions[0].Token, "MediaTool-GatewayCheck")
	if err != nil {
		return err
	}
	var item struct {
		ID      string `json:"Id"`
		Sources []struct {
			ID   string `json:"Id"`
			Path string `json:"Path"`
		} `json:"MediaSources"`
	}
	err = json.NewDecoder(io.LimitReader(itemResponse.Body, 1<<20)).Decode(&item)
	deferutil.Close(itemResponse.Body, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
	if err != nil || itemResponse.StatusCode != 200 || item.ID != in.ItemID {
		return errors.New("selected source unavailable")
	}
	fixed := ""
	for _, source := range item.Sources {
		if source.ID == in.SourceID {
			fixed = source.Path
		}
	}
	if fixed == "" {
		return errors.New("selected source missing")
	}
	grants := playbackgrants.Service{DB: db, Vault: vault, PublicURL: cfg.PublicURL}
	ref, err := grants.VerifyManagedURL(ctx, fixed)
	if err != nil {
		return err
	}
	assetID := ref.AssetID
	callerAgent := "MediaTool-GatewayCheck"
	if len(in.ClientAgents) > 0 {
		callerAgent = in.ClientAgents[0]
	}
	providerLink, err := (integrations.DynamicStorage{Manager: manager}).Resolve(ctx, storage.FileRef{AssetID: assetID, Path: ref.Path, UserAgent: callerAgent})
	if err != nil {
		return err
	}
	if export {
		policy := playback.Policy{BlockedHosts: []string{"185.99.135.205", "localhost", "server", "emby", "clouddrive2"}}
		if len(providerLink.Headers) != 0 || policy.Check(ctx, providerLink.URL) != nil {
			return errors.New("direct client capability unavailable")
		}
		// Explicit privileged diagnostic mode only. The caller must capture stdout
		// into memory; normal reports never include this URL.
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"url": providerLink.URL, "userAgent": providerLink.UserAgent})
	}
	agent := providerLink.UserAgent
	if agent == "" {
		agent = "MediaTool-GatewayCheck"
	}
	policy := playback.Policy{BlockedHosts: []string{"185.99.135.205", "localhost", "server", "emby", "clouddrive2"}}
	providerURL, _ := url.Parse(providerLink.URL)
	report := map[string]any{"checkedAt": time.Now().UTC().Format(time.RFC3339), "identitiesImported": imported, "realPlayerDecoded": false, "accountsModified": false, "providerRequiresUserAgent": providerLink.UserAgent != "", "officialProviderSelected": providerLink.DirectOnly && providerLink.CallerUserAgent, "callerUserAgent": callerAgent, "cdnHost": providerURL.Hostname()}

	// Bounded CDN probes distinguish an enforced UA binding from a suggested UA.
	probes := []map[string]any{}
	for _, clientAgent := range in.ClientAgents {
		if len(clientAgent) > 512 || strings.ContainsAny(clientAgent, "\r\n") {
			return errors.New("invalid diagnostic agent")
		}
		probe := map[string]any{"clientAgent": clientAgent}
		clientLink := providerLink
		if providerLink.CallerUserAgent {
			clientLink, err = (integrations.DynamicStorage{Manager: manager}).Resolve(ctx, storage.FileRef{AssetID: assetID, Path: ref.Path, UserAgent: clientAgent})
			if err != nil {
				probe["resolveSucceeded"] = false
				probes = append(probes, probe)
				continue
			}
		}
		u, _ := url.Parse(clientLink.URL)
		probe["cdnHost"] = u.Hostname()
		if policy.Check(ctx, clientLink.URL) == nil && len(clientLink.Headers) == 0 {
			res, e := request("GET", clientLink.URL, "", clientAgent)
			if e == nil {
				sample, readErr := io.ReadAll(io.LimitReader(res.Body, 1024))
				deferutil.Close(res.Body, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
				probe["status"] = res.StatusCode
				probe["sampleBytes"] = len(sample)
				probe["validPartialResponse"] = readErr == nil && res.StatusCode == 206 && len(sample) == 1024 && strings.HasPrefix(res.Header.Get("Content-Range"), "bytes 0-1023/")
			}
		}
		probes = append(probes, probe)
	}
	if len(probes) > 0 {
		report["cdnClientAgentProbes"] = probes
	}
	extension := strings.TrimPrefix(strings.ToLower(path.Ext(ref.Path)), ".")
	if !regexp.MustCompile(`^[a-z0-9]+$`).MatchString(extension) {
		return errors.New("invalid source extension")
	}
	query := url.Values{"Static": {"true"}, "MediaSourceId": {in.SourceID}, "Container": {extension}}
	target := strings.TrimRight(cfg.EmbyPublicURL, "/") + "/videos/" + in.ItemID + "/stream." + extension + "?" + query.Encode()
	record := func(name string, res *http.Response) {
		report[name+"Status"] = res.StatusCode
		report[name+"Gateway"] = res.Header.Get("X-Media-Gateway")
		report[name+"Reason"] = res.Header.Get("X-Media-Gateway-Reason")
		report[name+"Delivery"] = res.Header.Get("X-Media-Delivery")
		report[name+"DeliveryReason"] = res.Header.Get("X-Media-Delivery-Reason")
		report[name+"DirectValidated"] = res.Header.Get("X-Media-Direct-Verified") == "true"
		if raw := res.Header.Get("Location"); raw != "" {
			if u, err := url.Parse(raw); err == nil {
				report[name+"RedirectHost"] = u.Hostname()
			}
		}
	}
	originalURL := strings.TrimRight(cfg.EmbyPublicURL, "/") + "/videos/" + in.ItemID + "/original." + extension + "?" + url.Values{"MediaSourceId": {in.SourceID}}.Encode()
	native, err := request("HEAD", originalURL, in.Sessions[0].Token, agent)
	if err != nil {
		return err
	}
	record("nativeOriginal", native)
	report["nativeOriginal302Verified"] = native.StatusCode == 302 && native.Header.Get("X-Media-Gateway") == "emby-static" && policy.Check(ctx, native.Header.Get("Location")) == nil
	deferutil.Close(native.Body, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
	metadataURL := strings.TrimRight(cfg.EmbyPublicURL, "/") + "/Items/" + in.ItemID + "/PlaybackInfo?" + url.Values{"UserId": {in.Sessions[0].UserID}}.Encode()
	meta, err := request("GET", metadataURL, in.Sessions[0].Token, "SenPlayer/6.2.1")
	if err != nil {
		return err
	}
	var metadata struct {
		Sources []struct {
			ID              string            `json:"Id"`
			IsRemote        bool              `json:"IsRemote"`
			Protocol        string            `json:"Protocol"`
			Headers         map[string]string `json:"RequiredHttpHeaders"`
			DirectStreamURL string            `json:"DirectStreamUrl"`
		} `json:"MediaSources"`
	}
	err = json.NewDecoder(io.LimitReader(meta.Body, 1<<20)).Decode(&metadata)
	deferutil.Close(meta.Body, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
	if err == nil && meta.StatusCode == 200 {
		for _, source := range metadata.Sources {
			if source.ID == in.SourceID {
				report["nativeSourceIsRemote"] = source.IsRemote
				report["nativeSourceProtocol"] = source.Protocol
				report["nativeMetadataHeaderCount"] = len(source.Headers)
				report["nativeMetadataUAMatchesProvider"] = source.Headers["User-Agent"] == providerLink.UserAgent && providerLink.UserAgent != ""
				report["nativeHeaderOverrideUnneeded"] = providerLink.CallerUserAgent && len(source.Headers) == 0
				report["nativeRequiredUserAgent"] = source.Headers["User-Agent"]
				u, _ := url.Parse(source.DirectStreamURL)
				report["directStreamServerRelative"] = u != nil && u.Scheme == "" && u.Host == ""
			}
		}
	}
	compatible, err := request("GET", target, in.Sessions[0].Token, agent)
	if err != nil {
		return err
	}
	record("compatible", compatible)
	location := compatible.Header.Get("Location")
	deferutil.Close(compatible.Body, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
	safe := compatible.StatusCode == 302 && policy.Check(ctx, location) == nil
	report["authorizedStatic302Verified"] = safe && compatible.Header.Get("X-Media-Gateway") == "emby-static"
	if safe {
		media, e := request("GET", location, "", agent)
		if e == nil {
			sample, readErr := io.ReadAll(io.LimitReader(media.Body, 1024))
			deferutil.Close(media.Body, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
			digest := sha256.Sum256(sample)
			report["redirectMediaStatus"] = media.StatusCode
			report["redirectMediaBytes"] = len(sample)
			report["redirectSampleSha256"] = hex.EncodeToString(digest[:])
			report["redirectBytesVerified"] = readErr == nil && media.StatusCode == 206 && len(sample) == 1024
		}
	}
	ordinary, err := request("GET", target, in.Sessions[0].Token, "MediaTool-GatewayCheck")
	if err != nil {
		return err
	}
	record("ordinary", ordinary)
	if ordinary.StatusCode == 302 {
		location := ordinary.Header.Get("Location")
		deferutil.Close(ordinary.Body, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
		if policy.Check(ctx, location) != nil {
			return errors.New("unsafe ordinary redirect")
		}
		ordinary, err = request("GET", location, "", "MediaTool-GatewayCheck")
		if err != nil {
			return err
		}
	}
	data, readErr := io.ReadAll(io.LimitReader(ordinary.Body, 1024))
	deferutil.Close(ordinary.Body, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
	report["ordinaryBytesVerified"] = readErr == nil && ordinary.StatusCode == 206 && len(data) == 1024
	// Remote-seek probes are bounded: sample 1 KiB, never download the film.
	rangeChecks := []map[string]any{}
	for _, offset := range []int64{1 << 20, 256 << 20} {
		r, e := http.NewRequestWithContext(ctx, "GET", originalURL, nil)
		if e != nil {
			return e
		}
		r.Header.Set("X-Emby-Token", in.Sessions[0].Token)
		r.Header.Set("User-Agent", "MediaTool-GatewayCheck")
		r.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+1023))
		res, e := noRedirect.Do(r)
		check := map[string]any{"offset": offset}
		if e == nil && res.StatusCode == 302 {
			check["gatewayStatus"] = 302
			location := res.Header.Get("Location")
			deferutil.Close(res.Body, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
			if policy.Check(ctx, location) == nil {
				direct, _ := http.NewRequestWithContext(ctx, "GET", location, nil)
				direct.Header.Set("User-Agent", "MediaTool-GatewayCheck")
				direct.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+1023))
				res, e = noRedirect.Do(direct)
			} else {
				e = errors.New("unsafe range redirect")
			}
		}
		if e == nil {
			sample, readErr := io.ReadAll(io.LimitReader(res.Body, 1024))
			deferutil.Close(res.Body, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
			check["status"] = res.StatusCode
			check["bytes"] = len(sample)
			check["verified"] = readErr == nil && res.StatusCode == 206 && len(sample) == 1024 && strings.HasPrefix(res.Header.Get("Content-Range"), fmt.Sprintf("bytes %d-%d/", offset, offset+1023))
		}
		rangeChecks = append(rangeChecks, check)
	}
	if providerLink.DirectOnly {
		report["directNonzeroRangeChecks"] = rangeChecks
	} else {
		report["proxyNonzeroRangeChecks"] = rangeChecks
	}
	denied, err := request("HEAD", target, "invalid-session-token", agent)
	if err != nil {
		return err
	}
	report["invalidTokenStatus"] = denied.StatusCode
	report["invalidTokenRejected"] = denied.StatusCode == 401 && denied.Header.Get("Location") == ""
	deferutil.Close(denied.Body, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
	return json.NewEncoder(os.Stdout).Encode(report)
}

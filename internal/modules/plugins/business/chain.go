package business

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/acquisition"
	"media-tool/internal/modules/downloads"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/manualdownloads"
	"media-tool/internal/modules/mediaprobe/admin"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/networkproxy"
	"media-tool/internal/modules/search"
	"media-tool/internal/modules/storage"
	"media-tool/internal/modules/subscriptionpolicy"
	"media-tool/internal/modules/subscriptions"
	uuidcheck "media-tool/internal/platform/identity"
	"media-tool/internal/platform/secrets"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// AcquisitionHost is the sole business bridge. No SQL/engine/credential object is
// exported to workers. Network calls run outside the plugin authorization tx.
type AcquisitionHost struct {
	Network     *networkproxy.HostClient
	Processing  *admin.Repository
	Collector   stillCollector
	Observe     func(context.Context, string, string) (storage.File, error) // optional native observer; nil uses configured provider
	DB          *pgxpool.Pool
	Vault       *secrets.Vault
	Connections *integrations.Manager
	Redis       *redis.Client
}

func safeText(s string) string {
	s = sensitiveText.ReplaceAllString(s, "[redacted]")
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, s)
	r := []rune(s)
	if len(r) > 256 {
		s = string(r[:256])
	}
	return s
}
func decodeBusiness(raw json.RawMessage, out any) error {
	if contracts.Decode(raw, out) != nil {
		return contracts.NewError(contracts.InvalidParams)
	}
	return nil
}
func invalidBusiness() error     { return contracts.NewError(contracts.InvalidParams) }
func unsupportedBusiness() error { return contracts.NewError(contracts.Unsupported) }
func pageOK(p b.PageRequest) error {
	if !p.Valid() {
		return invalidBusiness()
	}
	return nil
}
func pageSlice[T any](v []T, p b.PageRequest) b.Page[T] {
	start := min((p.Page-1)*p.Size, len(v))
	end := min(start+p.Size, len(v))
	return b.Page[T]{Items: v[start:end], HasMore: end < len(v)}
}
func safeTask(v acquisition.Task) b.Task {
	return b.Task{SeedingState: v.SeedingState, ID: v.ID, Title: safeText(v.Title), State: safeState(v.State), Hash: v.InfoHash, Owned: true, Progress: v.Progress, DownloadSpeed: v.DownloadSpeed, UploadSpeed: v.UploadSpeed, Revision: v.SourceControlRevision, CleanupState: v.CleanupState}
}
func safeResource(v search.Resource) b.Resource {
	var hr *bool
	if h, ok := v.Metadata["hr"].(bool); ok {
		hr = &h
	}
	return b.Resource{ID: v.ID, AccountID: v.AccountID, Title: safeText(v.Title), Size: v.Size, Seeders: v.Seeders, Leechers: v.Leechers, HR: hr, DownloadFactor: v.DownloadFactor}
}
func nativeMedia(v b.Media) metadata.Item {
	return metadata.Item{ID: v.ID, MediaType: v.MediaType, Title: v.Title, OriginalTitle: v.OriginalTitle, Year: v.Year}
}
func safeMedia(v metadata.Item) b.Media {
	return b.Media{ID: v.ID, MediaType: v.MediaType, Title: safeText(v.Title), OriginalTitle: safeText(v.OriginalTitle), Year: v.Year}
}
func safeSubscription(v subscriptions.Record) b.Subscription {
	raw, _ := json.Marshal(v.Rules)
	var rules b.Rules
	_ = json.Unmarshal(raw, &rules)
	rules.Keyword = safeText(rules.Keyword)
	for i := range rules.Include {
		rules.Include[i] = safeText(rules.Include[i])
	}
	for i := range rules.Exclude {
		rules.Exclude[i] = safeText(rules.Exclude[i])
	}
	return b.Subscription{ID: v.ID, Title: safeText(v.Title), Media: safeMedia(v.Media), Enabled: v.Enabled, Source: v.Source, Rules: rules, Revision: v.Revision, State: v.LastState}
}
func businessID(raw json.RawMessage) (string, error) {
	var in b.ID
	if decodeBusiness(raw, &in) != nil || !validUUID(in.ID) {
		return "", invalidBusiness()
	}
	return in.ID, nil
}
func (h AcquisitionHost) engines() (search.Engine, acquisition.Engine, manualdownloads.Engine, subscriptions.Engine) {
	s := search.Engine{DB: h.DB, Redis: h.Redis, Vault: h.Vault, Connections: h.Connections}
	return s, acquisition.Engine{DB: h.DB, Connections: h.Connections, Search: s}, manualdownloads.Engine{DB: h.DB, Connections: h.Connections}, subscriptions.Engine{DB: h.DB, Connections: h.Connections}
}
func (h AcquisitionHost) Call(ctx context.Context, call contracts.HostCall, permit contracts.Permission, admit func() error) (any, error) {
	spec, ok := b.Lookup(call.Method)
	if !ok {
		return nil, unsupportedBusiness()
	}
	if !spec.Supported {
		return nil, unsupportedBusiness()
	}
	if platformMethod(call.Method) {
		return h.callPlatform(ctx, call, permit, admit)
	}
	if strings.HasPrefix(call.Method, "mediaserver.") || strings.HasPrefix(call.Method, "catalog.") || strings.HasPrefix(call.Method, "mediausers.") || strings.HasPrefix(call.Method, "playback.") || strings.HasPrefix(call.Method, "playeraccess.") || strings.HasPrefix(call.Method, "mediaprobe.") || call.Method == "metadata.nativeDetails" || call.Method == "metadata.refreshDetails" {
		return h.callServer(ctx, call, permit, admit)
	}
	media := strings.HasPrefix(call.Method, "library.") || strings.HasPrefix(call.Method, "classification.") || strings.HasPrefix(call.Method, "fs.media.") || strings.HasPrefix(call.Method, "storage.") || strings.HasPrefix(call.Method, "strm.") || strings.HasPrefix(call.Method, "metadata.") && call.Method != "metadata.torrent.parse"
	if media {
		if spec.Scope != permit.Scope {
			return nil, contracts.NewError(contracts.PermissionDenied)
		}
		if !mediaBounds(call.Args) {
			return nil, invalidBusiness()
		}
		switch {
		case strings.HasPrefix(call.Method, "fs.media."), strings.HasPrefix(call.Method, "storage."):
			return h.callFiles(ctx, call, permit, admit)
		case strings.HasPrefix(call.Method, "metadata."):
			return h.callMetadata(ctx, call, admit)
		case strings.HasPrefix(call.Method, "classification."):
			return h.callClassification(ctx, call, admit)
		default:
			return h.callLibrary(ctx, call, admit)
		}
	}
	se, ae, me, su := h.engines()

	ds := acquisition.DownloaderService{Engine: ae, AllowedDownloaders: permit.Resources["downloaderIds"]}
	if call.Method == "metadata.torrent.parse" {
		var in b.TorrentInput
		if decodeBusiness(call.Args, &in) != nil {
			return nil, invalidBusiness()
		}
		m, e := downloads.ParseTorrent(in.Data)
		if e != nil {
			return nil, invalidBusiness()
		}
		out := b.Torrent{Hash: m.InfoHash, Aliases: m.Aliases, Version: m.Version, Name: safeText(filepath.Base(m.Name)), Files: []b.File{}}
		if len(m.Files) > 200 {
			return nil, contracts.NewError(contracts.PayloadTooLarge)
		}
		for i, f := range m.Files {
			out.Files = append(out.Files, b.File{Index: i, Name: safeText(filepath.Base(f.Name)), Size: f.Size})
		}
		return out, nil
	}
	if call.Method == "subscriptions.evaluateRules" {
		var in b.Evaluation
		if decodeBusiness(call.Args, &in) != nil || validateRules(in.Rules) != nil {
			return nil, invalidBusiness()
		}
		var rules subscriptionpolicy.Rules
		raw, _ := json.Marshal(in.Rules)
		if json.Unmarshal(raw, &rules) != nil {
			return nil, invalidBusiness()
		}
		r := search.Resource{Title: in.Resource.Title, Size: in.Resource.Size, Seeders: in.Resource.Seeders, DownloadFactor: in.Resource.DownloadFactor, Metadata: map[string]any{}}
		if in.Resource.HR != nil {
			r.Metadata["hr"] = *in.Resource.HR
		}
		yes, reasons, _ := subscriptionpolicy.Evaluate(r, nativeMedia(in.Media), rules)
		for i := range reasons {
			reasons[i] = safeText(reasons[i])
		}
		return b.Evaluated{Accepted: yes, Reasons: reasons}, nil
	}
	if call.Method == "subscriptions.completion" {
		if permit.Scope != "subscriptions.read" {
			return nil, contracts.NewError(contracts.PermissionDenied)
		}
		var in b.ID
		if decodeBusiness(call.Args, &in) != nil || !contracts.OpaqueID(in.ID) {
			return nil, invalidBusiness()
		}
		if h.DB == nil || h.Connections == nil {
			return nil, contracts.NewError(contracts.Unavailable)
		}
		return (subscriptions.CoverageObserver{Engine: subscriptions.Engine{DB: h.DB, Connections: h.Connections}, Cache: h.Redis}).Check(ctx, in.ID)
	}
	if call.Method == "subscriptions.coverage" {
		var in b.CoverageInput
		if decodeBusiness(call.Args, &in) != nil || len(in.Names) > 200 {
			return nil, invalidBusiness()
		}
		for _, s := range in.Names {
			if utf8.RuneCountInString(s) > 200 {
				return nil, invalidBusiness()
			}
		}
		episodes, e := subscriptions.ManifestCoverage(in.Names, in.Season)
		return b.Coverage{Episodes: episodes, Evidence: "filename_only_not_subscription_completion"}, e
	}
	if h.Connections == nil || h.DB == nil {
		return nil, contracts.NewError(contracts.Unavailable)
	}
	if (call.Method == "downloads.downloader.allHashes" || call.Method == "downloads.downloader.inspect" || call.Method == "downloads.manualCreate" || call.Method == "downloads.addResource") && len(permit.Resources["downloaderIds"]) > 0 {
		_, c, e := h.Connections.QB(ctx)
		if e != nil {
			return nil, e
		}
		if !selected(permit, "downloaderIds", c.Record.ID) {
			return nil, contracts.NewError(contracts.PermissionDenied)
		}
	}
	switch call.Method {
	case "downloads.list", "downloads.manualList":
		var in b.PageRequest
		if decodeBusiness(call.Args, &in) != nil || pageOK(in) != nil {
			return nil, invalidBusiness()
		}
		out := b.Page[b.Task]{Items: []b.Task{}}
		if call.Method == "downloads.list" {
			v, e := ae.Page(ctx, (in.Page-1)*in.Size, in.Size, in.Query, "", permit.Resources["downloaderIds"])
			if e != nil {
				return nil, e
			}
			for _, t := range v {
				out.Items = append(out.Items, safeTask(t))
			}
		} else {
			v, e := me.Page(ctx, (in.Page-1)*in.Size, in.Size, in.Query, "", permit.Resources["downloaderIds"])
			if e != nil {
				return nil, e
			}
			for _, t := range v {
				out.Items = append(out.Items, b.Task{ID: t.ID, Title: safeText(t.Title), State: safeState(t.State), Hash: t.InfoHash, Owned: true, Progress: t.Progress, Size: t.TotalBytes, DownloadSpeed: t.DownloadSpeed, UploadSpeed: t.UploadSpeed})
			}
		}
		out.HasMore = len(out.Items) == in.Size
		return out, nil
	case "downloads.get", "downloads.progress", "downloads.cleanup.preview":
		id, e := businessID(call.Args)
		if e != nil {
			return nil, e
		}
		v, e := ae.Page(ctx, 0, 1, "", id, permit.Resources["downloaderIds"])
		if e != nil {
			return nil, e
		}
		if len(v) != 1 {
			return nil, contracts.NewError(contracts.NotFound)
		}
		if call.Method == "downloads.cleanup.preview" {
			state, hash, eligible, e := ds.PreviewCleanup(ctx, id)
			return b.Cleanup{ID: id, Hash: hash, Eligible: eligible, State: state, DeletesFiles: true}, e
		}
		out := safeTask(v[0])
		transfers, e := ds.Transfers(ctx, id)
		if e != nil {
			return nil, e
		}
		for _, t := range transfers {
			out.Transfers = append(out.Transfers, b.Transfer{ID: t.ID, State: t.State, Progress: t.Progress})
		}
		return out, nil
	case "downloads.addResource":
		var in b.AddResource
		if decodeBusiness(call.Args, &in) != nil || !validUUID(in.RunID) || !validUUID(in.ResourceID) || in.TMDBID < 1 || len(in.Episodes) > 200 {
			return nil, invalidBusiness()
		}
		if e := se.MediaResource(ctx, in.RunID, in.ResourceID); e != nil {
			return nil, e
		}
		r, e := se.RunResource(ctx, in.RunID, in.ResourceID)
		if e != nil {
			return nil, e
		}
		if hr, known := r.Metadata["hr"].(bool); !known || hr {
			return nil, contracts.NewError(contracts.NeedsAction)
		}
		if !selected(permit, "siteIds", r.AccountID) {
			return nil, contracts.NewError(contracts.PermissionDenied)
		}
		id, e := ae.StartExpectedFromRun(ctx, in.RunID, in.ResourceID, in.MediaType, in.TMDBID, in.ExpectedHash, in.Season, in.Episodes, "", 0)
		return b.Receipt{ID: id, State: "pending"}, e
	case "downloads.manualCreate":
		if len(permit.Resources["siteIds"]) > 0 {
			return nil, contracts.NewError(contracts.PermissionDenied)
		}
		var in b.ManualSource
		if decodeBusiness(call.Args, &in) != nil || (in.Magnet == "") == (len(in.Torrent) == 0) {
			return nil, invalidBusiness()
		}
		var source manualdownloads.Source
		var e error
		if in.Magnet != "" {
			m, err := manualdownloads.Magnet(in.Magnet)
			e = err
			source = manualdownloads.Source{Kind: "magnet", Label: "plugin magnet", URL: in.Magnet, Meta: m}
		} else {
			source, e = manualdownloads.File(in.Torrent, "plugin.torrent")
		}
		if e != nil {
			return nil, invalidBusiness()
		}
		id, e := me.Create(ctx, source)
		return b.Receipt{ID: id, State: "pending"}, e
	case "downloads.manualRetry":
		var in b.Retry
		if decodeBusiness(call.Args, &in) != nil || !validUUID(in.ID) {
			return nil, invalidBusiness()
		}
		if in.AcceptDuplicateRisk {
			return nil, unsupportedBusiness()
		}
		v, e := me.Page(ctx, 0, 1, "", in.ID, permit.Resources["downloaderIds"])
		if e != nil {
			return nil, e
		}
		if len(v) != 1 {
			return nil, contracts.NewError(contracts.PermissionDenied)
		}
		e = me.Retry(ctx, in.ID, true, false)
		return b.Receipt{ID: in.ID, State: "pending"}, e
	case "downloads.uploadControl":
		var in b.UploadControl
		if decodeBusiness(call.Args, &in) != nil || !validUUID(in.ID) {
			return nil, invalidBusiness()
		}
		if in.Action != "pause" && in.Action != "resume" && in.Action != "retry" {
			return nil, unsupportedBusiness()
		}
		e := ds.UploadControl(ctx, in.ID, in.Action, admit)
		return b.Receipt{ID: in.ID, State: "requested"}, e
	case "downloads.pause", "downloads.resume":
		id, e := businessID(call.Args)
		if e != nil {
			return nil, e
		}
		e = ds.SetPaused(ctx, id, call.Method == "downloads.pause", admit)
		return b.Receipt{ID: id, State: "requested"}, e
	case "downloads.cleanup.approve", "downloads.downloader.deleteOwned":
		var in b.DeleteRequest
		if decodeBusiness(call.Args, &in) != nil || !validUUID(in.ID) && !(call.Method == "downloads.downloader.deleteOwned" && validHash(in.ID)) {
			return nil, invalidBusiness()
		}
		id := in.ID
		eligible := true
		var e error
		if validUUID(id) {
			_, _, eligible, e = ds.PreviewCleanup(ctx, id)
		}
		if e != nil {
			return nil, e
		}
		if !eligible {
			return nil, contracts.NewError(contracts.NeedsAction)
		}
		e = ds.DeleteOwned(ctx, id, in.DeleteFiles, admit)
		if errors.Is(e, acquisition.ErrCleanupPrerequisites) {
			return nil, contracts.NewError(contracts.NeedsAction)
		}
		state := "retired"
		if in.DeleteFiles {
			state = "pending"
		}
		return b.Receipt{ID: id, State: state}, e
	case "downloads.downloader.configure":
		var in b.DownloaderConfig
		if decodeBusiness(call.Args, &in) != nil || !validUUID(in.ID) {
			return nil, invalidBusiness()
		}
		v := acquisition.Assistance{ID: in.ID, Tag: in.Tag, Category: in.Category, DownloadLimit: in.DownloadLimit, UploadLimit: in.UploadLimit, RatioLimit: in.RatioLimit, SeedingMinutes: in.SeedingMinutes}
		err := ds.Configure(ctx, v, admit)
		return b.Receipt{ID: in.ID, State: "configured"}, err
	case "downloads.recheck":
		id, err := businessID(call.Args)
		if err != nil {
			return nil, err
		}
		err = ds.Recheck(ctx, id, admit)
		return b.Receipt{ID: id, State: "requested"}, err
	case "downloads.downloader.setPaused":
		var in b.Paused
		if decodeBusiness(call.Args, &in) != nil || !validHash(in.Hash) {
			return nil, invalidBusiness()
		}
		e := ds.SetPaused(ctx, in.Hash, in.Paused, admit)
		return b.Receipt{State: "requested"}, e
	case "downloads.downloader.allHashes":
		var in b.PageRequest
		if decodeBusiness(call.Args, &in) != nil || pageOK(in) != nil {
			return nil, invalidBusiness()
		}
		v, e := ds.AllHashes(ctx)
		if e != nil {
			return nil, e
		}
		for _, hash := range v {
			if !validHash(hash) {
				return nil, contracts.NewError(contracts.InvalidExtensionResult)
			}
		}
		slices.Sort(v)
		return pageSlice(v, in), nil
	case "downloads.downloader.inspect":
		var in b.ID
		if decodeBusiness(call.Args, &in) != nil || !validHash(in.ID) {
			return nil, invalidBusiness()
		}
		v, e := ds.ObserveCleanup(ctx, in.ID)
		if e != nil {
			return nil, e
		}
		tags := []string{}
		for _, tag := range v.Tags {
			tags = append(tags, safeText(tag))
		}
		return b.Task{DownloadLimit: v.Task.DownloadLimit, UploadLimit: v.Task.UploadLimit, RatioLimit: v.Task.RatioLimit, SeedingMinutes: v.Task.SeedingMinutes, AutoManaged: v.Task.AutoManaged, ID: v.ID, SiteID: v.SiteID, Category: safeText(v.Task.Category), Tags: tags, Ratio: v.Task.Ratio, SeedingSeconds: v.Task.SeedingTime, Complete: v.Task.Complete, Seeding: v.Task.Seeding, CleanupEligible: v.Eligible, Title: safeText(v.Task.Name), State: safeState(v.Task.State), Hash: v.Task.ID, Owned: v.Owned, Progress: &v.Task.Progress, Size: v.Task.Size, DownloadSpeed: v.Task.DownloadSpeed, UploadSpeed: v.Task.UploadSpeed}, nil

	case "downloads.files":
		var in b.Selection
		if decodeBusiness(call.Args, &in) != nil || !validUUID(in.ID) || pageOK(in.PageRequest) != nil {
			return nil, invalidBusiness()
		}
		v, e := ds.Files(ctx, in.ID)
		if e != nil {
			return nil, e
		}
		out := []b.File{}
		if len(v) > 10000 {
			return nil, contracts.NewError(contracts.PayloadTooLarge)
		}
		for _, f := range v {
			out = append(out, b.File{Index: f.Index, Name: safeText(filepath.Base(f.Name)), Size: f.Size, Progress: f.Progress})
		}
		return pageSlice(out, in.PageRequest), nil
	case "downloads.trackers":
		id, e := businessID(call.Args)
		if e != nil {
			return nil, e
		}
		v, e := ds.Trackers(ctx, id)
		if e != nil {
			return nil, e
		}
		out := b.Page[b.Tracker]{Items: []b.Tracker{}}
		for _, t := range v {
			u, e := url.Parse(t.URL)
			origin := "[redacted]"
			if e == nil && u.Hostname() != "" {
				origin = u.Scheme + "://" + u.Hostname()
			}
			out.Items = append(out.Items, b.Tracker{Origin: origin, Status: t.Status, Seeds: t.Seeds, Peers: t.Peers})
		}
		return out, nil
	}
	return h.callOther(ctx, call, permit, admit, se, su)
}
func selected(p contracts.Permission, key, id string) bool {
	v, ok := p.Resources[key]
	return !ok || slices.Contains(v, id)
}
func validHash(s string) bool {
	ok, _ := regexp.MatchString(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`, s)
	return ok
}
func validateRules(r b.Rules) error {
	if len(r.AccountIDs) > 200 || len(r.Include) > 200 || len(r.Exclude) > 200 || len(r.Episodes) > 200 {
		return invalidBusiness()
	}
	for _, s := range append(append([]string{r.Keyword}, r.Include...), r.Exclude...) {
		if utf8.RuneCountInString(s) > 200 || !utf8.ValidString(s) {
			return invalidBusiness()
		}
	}
	raw, _ := json.Marshal(r)
	var v subscriptionpolicy.Rules
	if json.Unmarshal(raw, &v) != nil {
		return invalidBusiness()
	}
	return subscriptionpolicy.ValidateKeywords(v)
}

func validUUID(s string) bool { return uuidcheck.UUID(s) }

var sensitiveText = regexp.MustCompile(`(?i)(?:https?|magnet)\S+|(?:password|api.?key|passkey|cookie|authorization|token|pickcode)\s*[:=]\s*\S+`)

func safeState(s string) string {
	if len(s) > 64 || s == "" {
		return "unknown"
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_') {
			return "unknown"
		}
	}
	return s
}

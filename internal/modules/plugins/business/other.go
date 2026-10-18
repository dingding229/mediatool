package business

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"media-tool/internal/modules/search"
	"media-tool/internal/modules/sitemetrics"
	"media-tool/internal/modules/sites"
	"media-tool/internal/modules/subscriptions"
	"net/url"
	"strings"
)

func (h AcquisitionHost) callOther(ctx context.Context, call contracts.HostCall, p contracts.Permission, admit func() error, se search.Engine, su subscriptions.Engine) (any, error) {
	switch call.Method {
	case "search.start":
		var in b.SearchStart
		if decodeBusiness(call.Args, &in) != nil || len(in.Keyword) > 200 || in.Keyword == "" || len(in.AccountIDs) < 1 || len(in.AccountIDs) > 200 {
			return nil, invalidBusiness()
		}
		for _, id := range in.AccountIDs {
			if !validUUID(id) {
				return nil, invalidBusiness()
			}
			if !selected(p, "siteIds", id) {
				return nil, contracts.NewError(contracts.PermissionDenied)
			}
		}
		var id string
		var cached bool
		var e error
		if in.Media == nil {
			id, cached, e = se.StartCached(ctx, in.Keyword, in.AccountIDs, in.Refresh)
		} else {
			id, cached, e = se.StartMediaCached(ctx, in.Keyword, in.AccountIDs, in.Refresh, nativeMedia(*in.Media))
		}
		state := "pending"
		if cached {
			state = "cached"
		}
		return b.Receipt{ID: id, Cached: cached, State: state}, e
	case "search.list":
		var in b.PageRequest
		if decodeBusiness(call.Args, &in) != nil || pageOK(in) != nil {
			return nil, invalidBusiness()
		}
		v, e := se.Page(ctx, (in.Page-1)*in.Size, in.Size, in.Query, "")
		if e != nil {
			return nil, e
		}
		out := b.Page[b.SearchRun]{Items: []b.SearchRun{}, HasMore: len(v) == in.Size}
		for _, r := range v {
			out.Items = append(out.Items, b.SearchRun{ID: r.ID, Keyword: safeText(r.Keyword), State: safeState(r.State), Resources: []b.Resource{}})
		}
		return out, nil
	case "search.get", "search.resources":
		var in b.Selection
		if decodeBusiness(call.Args, &in) != nil || !validUUID(in.ID) || pageOK(in.PageRequest) != nil {
			return nil, invalidBusiness()
		}
		r, e := se.HostRun(ctx, in.ID, in.Page, in.Size, in.Query)
		if e != nil {
			return nil, e
		}
		out := b.SearchRun{ID: r.ID, Keyword: safeText(r.Keyword), State: safeState(r.State), Resources: []b.Resource{}, HasMore: in.Page*in.Size < r.Total}
		for _, v := range r.Resources {
			out.Resources = append(out.Resources, safeResource(v))
		}
		return out, nil
	case "sites.list", "sites.get", "sites.metrics":
		in := b.PageRequest{Page: 1, Size: 1}
		id := ""
		if call.Method == "sites.list" {
			if decodeBusiness(call.Args, &in) != nil || pageOK(in) != nil {
				return nil, invalidBusiness()
			}
		} else {
			var e error
			id, e = businessID(call.Args)
			if e != nil {
				return nil, e
			}
			if !selected(p, "siteIds", id) {
				return nil, contracts.NewError(contracts.PermissionDenied)
			}
		}
		rows, e := (sitemetrics.Engine{DB: h.DB, Connections: h.Connections}).Summaries(ctx, (in.Page-1)*in.Size, in.Size, in.Query, id, p.Resources["siteIds"])
		if e != nil {
			return nil, e
		}
		out := b.Page[b.Site]{Items: []b.Site{}, HasMore: len(rows) == in.Size}
		for _, v := range rows {
			out.Items = append(out.Items, b.Site{ID: v.ID, Name: safeText(v.Name), Enabled: v.Enabled, Revision: v.Revision, Origin: siteOrigin(v.Origin), TemplateID: safeText(v.TemplateID)})
		}
		if call.Method == "sites.list" {
			return out, nil
		}
		if len(rows) != 1 {
			return nil, contracts.NewError(contracts.NotFound)
		}
		if call.Method == "sites.get" {
			return out.Items[0], nil
		}
		v := rows[0]
		m := v.Metrics
		return b.Metrics{AccountID: v.ID, State: safeState(v.State), Uploaded: m.Uploaded, Downloaded: m.Downloaded, Ratio: m.Ratio, Bonus: m.Bonus, Seeding: m.Seeding, SeedingBytes: m.SeedingBytes}, nil
	case "sites.request", "sites.checkin":
		var in b.SiteRequest
		if decodeBusiness(call.Args, &in) != nil || !validUUID(in.AccountID) {
			return nil, invalidBusiness()
		}
		if in.Raw {
			return nil, unsupportedBusiness()
		}
		if !selected(p, "siteIds", in.AccountID) || !selected(p, "hosts", in.Origin) {
			return nil, contracts.NewError(contracts.PermissionDenied)
		}
		c, e := h.Connections.Site(ctx, in.AccountID)
		if e != nil {
			return nil, e
		}
		if e = admit(); e != nil {
			return nil, e
		}
		v, e := c.Controlled(ctx, sites.ControlledRequest{Origin: in.Origin, Path: in.Path, Method: in.Method, Body: in.Body, SuccessMarker: in.SuccessMarker})
		return b.SiteResponse{Status: v.Status, State: safeState(v.State), Matched: v.Matched}, e
	case "sites.metrics.collect", "sites.rss.discover", "sites.rss.refresh":
		id, e := businessID(call.Args)
		if e != nil {
			return nil, e
		}
		if !selected(p, "siteIds", id) {
			return nil, contracts.NewError(contracts.PermissionDenied)
		}
		if e = admit(); e != nil {
			return nil, e
		}
		if call.Method == "sites.metrics.collect" {
			e = (sitemetrics.Engine{DB: h.DB, Connections: h.Connections}).Collect(ctx, id)
		} else {
			c, err := h.Connections.Site(ctx, id)
			if err != nil {
				return nil, err
			}
			if call.Method == "sites.rss.discover" {
				_, _, e = c.DiscoverRSS(ctx)
			} else {
				secret, err := h.Connections.Repository.Get(ctx, id)
				if err != nil {
					return nil, err
				}
				_, e = c.RSS(ctx, secret.Values["rssUrl"])
			}
		}
		return b.Receipt{ID: id, State: "observed"}, e
	case "sites.templates.catalog", "sites.templates.loaded":
		var in b.PageRequest
		if decodeBusiness(call.Args, &in) != nil || pageOK(in) != nil {
			return nil, invalidBusiness()
		}
		loaded, e := (sites.Repository{DB: h.DB}).LoadedGoTemplates(ctx)
		if e != nil {
			return nil, e
		}
		out := []b.Template{}
		for _, t := range sites.GoTemplates() {
			if !selected(p, "siteIds", t.ID) || in.Query != "" && !strings.Contains(strings.ToLower(t.Name), strings.ToLower(in.Query)) {
				continue
			}
			if call.Method == "sites.templates.loaded" && !loaded[t.ID] {
				continue
			}
			out = append(out, b.Template{ID: t.ID, Name: safeText(t.Name), Loaded: loaded[t.ID]})
		}
		return pageSlice(out, in), nil
	case "sites.templates.load":
		var in b.IDs
		if decodeBusiness(call.Args, &in) != nil || len(in.IDs) < 1 || len(in.IDs) > 200 {
			return nil, invalidBusiness()
		}
		for _, id := range in.IDs {
			if !selected(p, "siteIds", id) {
				return nil, contracts.NewError(contracts.PermissionDenied)
			}
		}
		e := (sites.Repository{DB: h.DB}).LoadGoTemplates(ctx, in.IDs)
		return b.Receipt{State: "selected"}, e
	case "subscriptions.list":
		var in b.PageRequest
		if decodeBusiness(call.Args, &in) != nil || pageOK(in) != nil {
			return nil, invalidBusiness()
		}
		v, e := su.Page(ctx, (in.Page-1)*in.Size, in.Size, in.Query, "")
		if e != nil {
			return nil, e
		}
		out := b.Page[b.Subscription]{Items: []b.Subscription{}, HasMore: len(v) == in.Size}
		for _, r := range v {
			out.Items = append(out.Items, safeSubscription(r))
		}
		return out, nil
	case "subscriptions.get":
		id, e := businessID(call.Args)
		if e != nil {
			return nil, e
		}
		v, e := su.Get(ctx, id)
		return safeSubscription(v), e
	case "subscriptions.history":
		var in b.Selection
		if decodeBusiness(call.Args, &in) != nil || !validUUID(in.ID) || pageOK(in.PageRequest) != nil {
			return nil, invalidBusiness()
		}
		v, e := su.HistoryPage(ctx, in.ID, (in.Page-1)*in.Size, in.Size)
		if e != nil {
			return nil, e
		}
		out := b.Page[b.History]{Items: []b.History{}, HasMore: len(v) == in.Size}
		for _, r := range v {
			out.Items = append(out.Items, b.History{ID: r.ID, State: safeState(r.State), Preview: r.Preview})
		}
		return out, nil
	case "subscriptions.create", "subscriptions.update":
		var in b.SubscriptionInput
		if decodeBusiness(call.Args, &in) != nil || validateRules(in.Rules) != nil || call.Method == "subscriptions.create" && in.ID != "" || call.Method == "subscriptions.update" && (!validUUID(in.ID) || call.ExpectedRevision == nil) {
			return nil, invalidBusiness()
		}
		if in.ID != "" {
			old, e := su.Get(ctx, in.ID)
			if e != nil {
				return nil, e
			}
			for _, id := range old.Rules.AccountIDs {
				if !selected(p, "siteIds", id) {
					return nil, contracts.NewError(contracts.PermissionDenied)
				}
			}
		}
		for _, id := range in.Rules.AccountIDs {
			if !selected(p, "siteIds", id) {
				return nil, contracts.NewError(contracts.PermissionDenied)
			}
		}
		raw, _ := json.Marshal(in.Rules)
		var rules subscriptions.Rules
		if json.Unmarshal(raw, &rules) != nil {
			return nil, invalidBusiness()
		}
		input := subscriptions.Input{MediaType: in.MediaType, TMDBID: in.TMDBID, Enabled: in.Enabled, Source: in.Source, Rules: rules}
		if call.ExpectedRevision != nil {
			input.Revision = *call.ExpectedRevision
		}
		v, e := su.Save(ctx, in.ID, input)
		return safeSubscription(v), e
	case "subscriptions.queue":
		var in b.Queue
		if decodeBusiness(call.Args, &in) != nil || !validUUID(in.ID) {
			return nil, invalidBusiness()
		}
		existing, e := su.Get(ctx, in.ID)
		if e != nil {
			return nil, e
		}
		for _, id := range existing.Rules.AccountIDs {
			if !selected(p, "siteIds", id) {
				return nil, contracts.NewError(contracts.PermissionDenied)
			}
		}
		id, e := su.Queue(ctx, in.ID, in.Preview)
		return b.Receipt{ID: id, State: "pending"}, e
	case "subscriptions.delete":
		id, e := businessID(call.Args)
		if e != nil || call.ExpectedRevision == nil {
			return nil, invalidBusiness()
		}
		existing, err := su.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		for _, account := range existing.Rules.AccountIDs {
			if !selected(p, "siteIds", account) {
				return nil, contracts.NewError(contracts.PermissionDenied)
			}
		}
		e = su.DeleteRevision(ctx, id, *call.ExpectedRevision)
		return b.Receipt{ID: id, State: "archived"}, e
	}
	return nil, unsupportedBusiness()
}

func siteOrigin(raw string) string {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || u.Scheme != "https" && u.Scheme != "http" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

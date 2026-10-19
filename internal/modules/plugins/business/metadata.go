package business

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"media-tool/internal/modules/classification"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/nativelibrary/catalog"
	"media-tool/internal/modules/nativetmdb"
	"media-tool/internal/modules/releaseattrs"
	"media-tool/internal/modules/releasename"
)

// copyDTO is used only between explicitly enumerated, pure public DTOs; never
// marshal an engine, credential record, generic map or raw provider response.
func copyDTO[T any](in any) T {
	var out T
	raw, _ := json.Marshal(in)
	_ = json.Unmarshal(raw, &out)
	return out
}
func safeHint(h releasename.Hint) b.NameHint {
	v := copyDTO[b.NameHint](h)
	v.Title = safeText(v.Title)
	v.Group = safeText(v.Group)
	return v
}
func (h AcquisitionHost) callMetadata(ctx context.Context, call contracts.HostCall, admit func() error) (any, error) {
	switch call.Method {
	case "metadata.wishlist":
		return h.callWishlist(ctx, call)
	case "metadata.parse", "metadata.attributes.parse", "metadata.episodes":
		var in b.Text
		if decodeBusiness(call.Args, &in) != nil {
			return nil, invalidBusiness()
		}
		switch call.Method {
		case "metadata.wishlist":
			return h.callWishlist(ctx, call)
		case "metadata.parse":
			return safeHint(releasename.Parse(in.Text)), nil
		case "metadata.attributes.parse":
			return releaseattrs.Parse(in.Text), nil
		default:
			s, eps, ok := metadata.Episodes(in.Text)
			if len(eps) > 200 {
				return nil, contracts.NewError(contracts.PayloadTooLarge)
			}
			return b.EpisodeCoverage{Season: s, Episodes: eps, Valid: ok}, nil
		}
	case "metadata.attributes.validate":
		var in b.Attribute
		if decodeBusiness(call.Args, &in) != nil {
			return nil, invalidBusiness()
		}
		return releaseattrs.Valid(in.Key, in.Value), nil
	case "metadata.normalize":
		var in b.EpisodeCoverage
		if decodeBusiness(call.Args, &in) != nil {
			return nil, invalidBusiness()
		}
		v, err := metadata.NormalizeEpisodes(in.Season, in.Episodes)
		return b.EpisodeCoverage{Season: in.Season, Episodes: v, Valid: err == nil}, err
	case "metadata.name.render":
		var in b.RenderName
		if decodeBusiness(call.Args, &in) != nil {
			return nil, invalidBusiness()
		}
		hint := copyDTO[releasename.Hint](in.Hint)
		if in.Path {
			v, err := releasename.RenderPath(in.Template, hint, in.Original)
			return safeText(v), err
		}
		v, err := releasename.Render(in.Template, hint, in.Original)
		return safeText(v), err
	case "metadata.search", "metadata.details", "metadata.storedDetails":
		var in b.MetadataQuery
		if decodeBusiness(call.Args, &in) != nil || in.MediaType != "movie" && in.MediaType != "series" && in.MediaType != "season" {
			return nil, invalidBusiness()
		}
		if h.Connections == nil {
			return nil, unsupportedBusiness()
		}
		if call.Method == "metadata.storedDetails" {
			client, err := h.Connections.TMDB(ctx)
			if err != nil {
				return nil, err
			}
			lang := client.Language
			if lang == "" {
				lang = "zh-CN"
			}
			v, state, err := (nativetmdb.Repository{DB: h.DB}).Stored(ctx, nativetmdb.Target{Type: in.MediaType, TMDBID: int64(in.ID), Season: in.Season}, lang)
			return b.StoredMetadata{State: state, Media: b.Media{ID: in.ID, MediaType: in.MediaType, Title: safeText(v.Name)}, Overview: safeText(v.Overview), RuntimeMinutes: v.RuntimeMinutes}, err
		}
		if in.MediaType == "season" {
			return nil, unsupportedBusiness()
		}
		client, err := h.Connections.TMDB(ctx)
		if err != nil {
			return nil, err
		}
		if call.Method == "metadata.details" {
			if in.ID < 1 {
				return nil, invalidBusiness()
			}
			v, err := client.Details(ctx, in.MediaType, in.ID)
			return safeMedia(v), err
		}
		if in.Query == "" {
			return nil, invalidBusiness()
		}
		v, err := client.Search(ctx, in.MediaType, in.Query)
		if err != nil {
			return nil, err
		}
		if len(v) > 200 {
			return nil, contracts.NewError(contracts.PayloadTooLarge)
		}
		out := []b.Media{}
		for _, i := range v {
			out = append(out, safeMedia(i))
		}
		return out, nil
	case "metadata.refresh":
		id, err := businessID(call.Args)
		if err != nil {
			return nil, err
		}
		if h.Connections == nil {
			return nil, unsupportedBusiness()
		}
		lang, err := (nativetmdb.Engine{DB: h.DB, Connections: h.Connections}).Language(ctx)
		if err != nil {
			return nil, err
		}
		if err = admit(); err != nil {
			return nil, err
		}
		err = (nativetmdb.Repository{DB: h.DB}).EnqueueAsset(ctx, id, lang)
		return b.Receipt{ID: id, State: "requested"}, err
	}
	return nil, unsupportedBusiness()
}
func (h AcquisitionHost) callClassification(ctx context.Context, call contracts.HostCall, admit func() error) (any, error) {
	if call.Method == "classification.update" {
		var in b.Classification
		if decodeBusiness(call.Args, &in) != nil || call.ExpectedRevision == nil || *call.ExpectedRevision != in.Revision {
			return nil, invalidBusiness()
		}
		policy := copyDTO[classification.Policy](in.Value)
		if err := policy.Validate(); err != nil {
			return nil, invalidBusiness()
		}
		if err := admit(); err != nil {
			return nil, err
		}
		out, err := (catalog.Repository{DB: h.DB}).SavePolicy(ctx, policy, in.Revision)
		return copyDTO[b.Classification](out), err
	}
	record, err := (classification.Repository{DB: h.DB}).Get(ctx)
	if err != nil {
		return nil, err
	}
	if call.Method == "classification.get" {
		var in b.Empty
		if decodeBusiness(call.Args, &in) != nil {
			return nil, invalidBusiness()
		}
		return copyDTO[b.Classification](record), nil
	}
	var in b.Classify
	if decodeBusiness(call.Args, &in) != nil {
		return nil, invalidBusiness()
	}
	item := nativeMedia(in.Media)
	item.GenreIDs = in.GenreIDs
	item.Countries = in.Countries
	item.OriginalLanguage = in.OriginalLanguage
	if call.Method == "classification.match" {
		return copyDTO[b.Category](record.Value.Match(item)), nil
	}
	if call.Method == "classification.render" {
		v, _, err := classification.Render(record.Value, item, in.Source, in.MovieTemplate, in.SeriesTemplate, in.Extension, in.Season, in.Episodes)
		return safeText(v), err
	}
	return nil, unsupportedBusiness()
}

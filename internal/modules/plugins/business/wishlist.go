package business

import (
	"context"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"media-tool/internal/modules/wishlist"
)

func (h AcquisitionHost) callWishlist(ctx context.Context, call contracts.HostCall) (any, error) {
	var in b.WishlistRequest
	if decodeBusiness(call.Args, &in) != nil {
		return nil, invalidBusiness()
	}
	if in.Source != "tmdb-list" && in.Source != "douban-wish" && in.Source != "douban-subject" {
		return nil, unsupportedBusiness()
	}
	if in.Page < 1 || in.Page > 1000 || len(in.ID) > 64 {
		return nil, invalidBusiness()
	}
	if h.Connections == nil {
		return nil, unsupportedBusiness()
	}
	v, e := (wishlist.Service{Connections: h.Connections}).Fetch(ctx, wishlist.Request{Source: in.Source, ID: in.ID, Page: in.Page})
	if e != nil {
		return nil, contracts.NewError(contracts.Unavailable)
	}
	out := b.WishlistPage{Items: []b.WishlistEntry{}, State: v.State, HasMore: v.HasMore, RetryAfterSeconds: v.RetryAfterSeconds}
	if len(v.Items) > 20 {
		return nil, contracts.NewError(contracts.PayloadTooLarge)
	}
	for _, x := range v.Items {
		out.Items = append(out.Items, b.WishlistEntry{SourceID: safeText(x.SourceID), Title: safeText(x.Title), Year: x.Year, MediaType: x.MediaType, TMDBID: x.TMDBID})
	}
	return out, nil
}

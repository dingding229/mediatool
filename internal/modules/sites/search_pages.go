package sites

import (
	"context"
	"errors"
	"net/url"
)

var ErrSearchResponse = errors.New("站点搜索响应没有有效的声明结果列表，请检查登录和接口版本")

const SearchResourceLimit = 500
const SearchPageLimit = 5

type SearchPageResult struct {
	Target, Next            *url.URL
	LinkPagination, LinkEnd bool
	LinkError               error
	Resources               []Resource
	RawRows                 int
	Truncated               bool
}
type SearchResult struct {
	Resources []Resource
	Pages     int
	Complete  bool   // Complete within the executed request contract, not the whole tracker.
	Reason    string // Fixed enums only; no URL/keyword/provider error content.
}

// SearchAll follows only a declared page placeholder or the existing mTorrent
// fallback. No page parameter is invented for a single-page site definition.
// It never guesses a page's starting offset, time-zone, total or page size.
func (c Client) SearchAll(ctx context.Context, q SearchQuery) (SearchResult, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	result := SearchResult{Resources: []Resource{}}
	paged := c.searchHasPagination()
	seen := map[string]bool{}
	visited := map[string]bool{}
	var next *url.URL
	for count := 0; count < SearchPageLimit; count++ {
		page, err := c.searchPageLink(ctx, q, next, count == 0)
		if err != nil {
			if len(result.Resources) == 0 || errors.Is(err, ErrAuthentication) || errors.Is(ctx.Err(), context.Canceled) {
				if ctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled) {
					err = ctx.Err()
				}
				return SearchResult{Resources: []Resource{}, Pages: result.Pages}, err
			}
			result.Reason = "later_page_failed"
			return result, nil
		}
		result.Pages++
		if page.Target != nil {
			visited[page.Target.String()] = true
		}

		added := 0
		for _, item := range page.Resources {
			if seen[item.TorrentID] {
				continue
			}
			if len(result.Resources) == SearchResourceLimit {
				result.Reason = "resource_limit"
				return result, nil
			}
			seen[item.TorrentID] = true
			result.Resources = append(result.Resources, item)
			added++
		}
		if page.Truncated {
			result.Reason = "resource_limit"
			return result, nil
		}
		if page.LinkError != nil {
			result.Reason = "invalid_next_page"
			return result, nil
		}
		if page.LinkPagination {
			if page.Next != nil {
				if visited[page.Next.String()] || added == 0 {
					result.Reason = "repeated_page"
					return result, nil
				}
				next = page.Next
				q.Page++
				continue
			}
			if page.LinkEnd {
				result.Complete = true
				result.Reason = "last_page"
				return result, nil
			}
		}
		if !paged {
			// A valid empty list is the declared query response. Nonempty
			// unpaged data cannot prove coverage beyond the returned page.
			result.Complete = page.RawRows == 0
			result.Reason = "single_page_contract"
			return result, nil
		}
		if page.RawRows == 0 {
			result.Complete = true
			result.Reason = "empty_page"
			return result, nil
		}
		if added == 0 {
			result.Reason = "repeated_page"
			return result, nil
		}
		q.Page++
	}
	result.Reason = "page_limit"
	return result, nil
}

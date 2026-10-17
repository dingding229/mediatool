package httpapi

import (
	"media-tool/internal/modules/mediausers"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
	"net/http"
)

type nativeLibraryMapping struct {
	ID             string                       `json:"id"`
	ItemID         string                       `json:"itemId"`
	Name           string                       `json:"name"`
	CollectionType nativelibrary.CollectionType `json:"collectionType"`
	Hidden         bool                         `json:"hidden"`
	SortOrder      int                          `json:"sortOrder"`
	GroupID        string                       `json:"groupId"`
	RuleID         string                       `json:"ruleId"`
	GroupName      string                       `json:"groupName"`
	RuleName       string                       `json:"ruleName"`
	AssetCount     int64                        `json:"assetCount"`
}

func (a *API) nativeLibraryMappings(w http.ResponseWriter, r *http.Request) {
	start, limit, ok := managementPagination(w, r)
	if !ok {
		return
	}
	var reader NativeLibraryReader = a.Deps.NativeLibraries
	if reader == nil {
		reader = catalog.Repository{DB: a.Deps.DB}
	}
	page, err := reader.List(r.Context(), start, limit)
	if err != nil {
		fail(w, 503, "native_libraries_unavailable", "库分类映射暂不可用")
		return
	}
	out := mediausers.ManagementPage[nativeLibraryMapping]{Items: []nativeLibraryMapping{}, Total: int(page.Total), StartIndex: start, Limit: limit}
	for _, v := range page.Items {
		out.Items = append(out.Items, nativeLibraryMapping{v.ID, v.ItemID, mediausers.ManagementLabel(v.Name), v.CollectionType, v.Hidden, v.SortOrder, v.GroupID, v.RuleID, mediausers.ManagementLabel(v.GroupName), mediausers.ManagementLabel(v.RuleName), v.AssetCount})
	}
	write(w, 200, out)
}

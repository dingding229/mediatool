package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"context"
	"media-tool/internal/modules/mediausers"
	"media-tool/internal/modules/nativelibrary/catalog"
	"media-tool/internal/modules/settings"
)

type managementPageRequest struct {
	UserID     string `json:"userId"`
	StartIndex int    `json:"startIndex"`
	Limit      int    `json:"limit"`
}
type managementContract struct {
	Devices struct {
		Request  managementPageRequest                                          `json:"request"`
		Response contractResponse[mediausers.ManagementPage[mediausers.Device]] `json:"response"`
	} `json:"devices"`
	History struct {
		Request  managementPageRequest                                               `json:"request"`
		Response contractResponse[mediausers.ManagementPage[mediausers.HistoryItem]] `json:"response"`
	} `json:"history"`
	Revoke struct {
		Request struct {
			UserID   string `json:"userId"`
			DeviceID string `json:"deviceId"`
		} `json:"request"`
		Response contractResponse[struct {
			Revoked bool `json:"revoked"`
		}] `json:"response"`
	} `json:"revoke"`
	Settings struct {
		Request  settings.NativeManagement                   `json:"request"`
		Response contractResponse[nativeManagementInfo]      `json:"response"`
		Saved    contractResponse[settings.NativeManagement] `json:"saved"`
	} `json:"settings"`
	Mappings struct {
		Request struct {
			StartIndex int `json:"startIndex"`
			Limit      int `json:"limit"`
		} `json:"request"`
		Response contractResponse[mediausers.ManagementPage[nativeLibraryMapping]] `json:"response"`
	} `json:"mappings"`
}

func readManagementContract(t *testing.T) managementContract {
	t.Helper()
	raw, err := os.ReadFile("testdata/contracts/native-management.json")
	if err != nil {
		t.Fatal(err)
	}
	var c managementContract
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&c); err != nil {
		t.Fatal(err)
	}
	return c
}
func TestNativeManagementContractsPaginationAndPortBoundary(t *testing.T) {
	c := readManagementContract(t)
	if !c.Settings.Request.Valid() || c.Devices.Response.Data.Items[0].ActiveTokens != 1 || !c.Revoke.Response.Data.Revoked || c.History.Response.Data.Items[0].ItemID != "100043" {
		t.Fatal("contract drift")
	}
	a := &API{}
	for _, query := range []string{"?limit=201", "?limit=0", "?limit=1&limit=2", "?startIndex=-1", "?startIndex=1000001", "?limit=NaN"} {
		for _, handler := range []func(*httptest.ResponseRecorder){func(w *httptest.ResponseRecorder) { a.mediaUserDevices(w, httptest.NewRequest("GET", "/"+query, nil)) }, func(w *httptest.ResponseRecorder) { a.mediaUserHistory(w, httptest.NewRequest("GET", "/"+query, nil)) }, func(w *httptest.ResponseRecorder) {
			a.nativeLibraryMappings(w, httptest.NewRequest("GET", "/"+query, nil))
		}} {
			w := httptest.NewRecorder()
			handler(w)
			if w.Code != 400 {
				t.Fatal(query, w.Code)
			}
		}
	}
	admin := New(cfg(), Dependencies{})
	member, err := NativeMediaHandler(cfg(), Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []struct{ method, path string }{{"GET", "/api/v1/media-server/users/" + c.Devices.Request.UserID + "/devices"}, {"GET", "/api/v1/media-server/users/" + c.History.Request.UserID + "/history"}, {"POST", "/api/v1/media-server/users/" + c.Revoke.Request.UserID + "/devices/" + c.Revoke.Request.DeviceID + "/revoke"}, {"GET", "/api/v1/media-server/native/settings"}, {"PUT", "/api/v1/media-server/native/settings"}, {"GET", "/api/v1/media-server/native/library-mappings"}} {
		w := httptest.NewRecorder()
		admin.ServeHTTP(w, httptest.NewRequest(route.method, route.path, nil))
		if w.Code != 401 {
			t.Fatal(route, w.Code)
		}
		w = httptest.NewRecorder()
		member.ServeHTTP(w, httptest.NewRequest(route.method, route.path, nil))
		if w.Code != 404 {
			t.Fatal("media port registered admin", route, w.Code)
		}
	}
	// Map by an explicit allowlist; even an accidentally populated physical directory
	// from an older repository cannot reach this new management response.
	a.Deps.NativeLibraries = nativeLibraryReaderFunc(func(context.Context, int, int) (catalog.Page, error) {
		v := c.Mappings.Response.Data.Items[0]
		return catalog.Page{Total: 1, Items: []catalog.Library{{ID: v.ID, ItemID: v.ItemID, Name: v.Name, Directory: "/115/private-marker", CollectionType: v.CollectionType, GroupID: v.GroupID, GroupName: v.GroupName, AssetCount: 1}}}, nil
	})
	w := httptest.NewRecorder()
	a.nativeLibraryMappings(w, httptest.NewRequest("GET", "/?limit=50", nil))
	if w.Code != 200 || !bytes.Equal(mustJSON(c.Mappings.Response), bytes.TrimSpace(w.Body.Bytes())) {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestNativeManagementAddressNeverReflectsSensitiveComponents(t *testing.T) {
	for _, v := range []struct{ raw, want string }{{"https://native.synthetic.invalid/", "https://native.synthetic.invalid"}, {"http://[::1]:8092", "http://[::1]:8092"}, {"https://user:secret@native.synthetic.invalid", ""}, {"https://native.synthetic.invalid?api_key=secret", ""}, {"https://native.synthetic.invalid/115/private", ""}, {"https://native.synthetic.invalid/#secret", ""}, {"https://native.synthetic.invalid?", ""}, {"file:///private/path", ""}} {
		out, ok := safeNativeAddress(v.raw)
		if out != v.want || ok != (v.want != "") {
			t.Fatal(v.raw, out, ok)
		}
	}
}

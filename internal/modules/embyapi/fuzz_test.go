package embyapi

import (
	"net/http/httptest"
	"testing"
)

func FuzzAuthAndQueryNeverPanic(f *testing.F) {
	for _, seed := range [][2]string{{`MediaBrowser Client="Infuse",Device="TV, Room",DeviceId="device",Token="owned"`, "ApiKey=owned&Limit=100"}, {`Emby Client=SenPlayer`, ""}, {`Client="broken`, "%ZZ"}, {"", "SortBy=SortName&IncludeItemTypes=Movie"}} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, header, query string) {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("X-Emby-Authorization", header)
		params, err := ParseValues(query)
		if err != nil {
			return
		}
		auth, err := ParseAuth(r, params)
		if err == nil && (!validText(auth.Client, 128) || !validText(auth.DeviceID, 256) || !validText(auth.Version, 64)) {
			t.Fatal("unbounded auth")
		}
		parsed, err := ParseQuery(params)
		if err == nil && (parsed.Limit < 0 || parsed.Limit > 200 || len(parsed.IDs) > 200) {
			t.Fatal("unbounded query")
		}
	})
}

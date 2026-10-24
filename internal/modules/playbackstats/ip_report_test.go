package playbackstats

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/ipgeo"
	"media-tool/internal/modules/jobs"
)

func reportLocation(ip string) ipgeo.Location {
	if ip == "223.5.5.5" {
		return ipgeo.Location{IP: ip, Country: "中国", Province: "浙江省", City: "杭州市", ISP: "阿里", Label: "中国 浙江省 杭州市 阿里"}
	}
	return ipgeo.Location{IP: ip, Country: "中国", Province: "广东省", City: "深圳市", ISP: "电信", Label: "中国 广东省 深圳市 电信"}
}
func TestIPReportFullSetKPIsSearchPaginationAndHints(t *testing.T) {
	now := time.Now()
	base := []IPRow{
		{IP: "223.5.5.5", Count: 12, FirstSeenAt: now.Add(-30 * 24 * time.Hour), LastSeenAt: now, Accounts: []IPAccount{{"one", "家人"}, {"two", "家人"}}},
		{IP: "240e:3b7:3272::1", Count: 3, FirstSeenAt: now.Add(-time.Hour), LastSeenAt: now, Accounts: []IPAccount{{"one", "家人"}}, Clients: []IPClient{{"device", "手机", "SenPlayer"}}},
		{IP: "10.0.0.1", Count: 99, Accounts: []IPAccount{{"private", "内网"}}},
	}
	f := Filter{Days: 7, Limit: 1, Start: 1, UserID: "11111111-1111-4111-8111-111111111111"}
	out := buildIPReport(base, f, reportLocation, now)
	if out.Total != 2 || out.Summary.UniqueIPs != 2 || out.Summary.ActiveUsers != 2 || out.Summary.Regions != 2 || len(out.Items) != 1 || out.Items[0].IP != base[1].IP || !out.Items[0].NewIP || !out.Items[0].DifferentRegion {
		t.Fatalf("%+v", out)
	}
	f.Start = 0
	f.Query = "深圳"
	out = buildIPReport(base, f, reportLocation, now)
	if out.Total != 1 || out.Summary.ActiveUsers != 1 || !out.Items[0].DifferentRegion || out.Summary.PrimaryRegion != "中国 浙江省 杭州市" {
		t.Fatal(out)
	}
	f.Query = "senplayer"
	if buildIPReport(base, f, reportLocation, now).Total != 1 {
		t.Fatal("client search")
	}
	f.Query = "家人"
	if buildIPReport(base, f, reportLocation, now).Total != 2 {
		t.Fatal("user search")
	}
	f.Query = "none"
	out = buildIPReport(base, f, reportLocation, now)
	if out.Total != 0 || len(out.Items) != 0 || out.Summary.UniqueIPs != 0 {
		t.Fatal(out)
	}
	f.Query = ""
	f.Limit = 25
	out = buildIPReport(base, f, reportLocation, now)
	if out.Items[0].NewIP {
		t.Fatal("window must not relabel an old IP as new")
	}
	if (Filter{Days: 7, Limit: 25, Query: strings.Repeat("省", 101)}).Validate() == nil {
		t.Fatal("unbounded search")
	}
}
func TestIPReportRealAggregationRetainsFirstAndDeduplicatesUsers(t *testing.T) {
	db := testPool(t)
	ctx := t.Context()
	users := []string{jobs.UUID(), jobs.UUID()}
	devices := []string{jobs.UUID(), jobs.UUID()}
	for i, id := range users {
		if _, e := db.Exec(ctx, `INSERT INTO media_users(id,username,display_name,password_hash,must_change_password)VALUES($1,$2,$3,'synthetic',false)`, id, "h2a-"+id, "家人"); e != nil {
			t.Fatal(e)
		}
		if _, e := db.Exec(ctx, `INSERT INTO media_user_devices(id,user_id,device_key,name,client)VALUES($1,$2,'h2a-device','客厅设备','SenPlayer')`, devices[i], id); e != nil {
			t.Fatal(e)
		}
	}
	t.Cleanup(func() {
		if _, e := db.Exec(context.Background(), `DELETE FROM media_users WHERE id=ANY($1::uuid[])`, users); e != nil {
			t.Error(e)
		}
	})
	recorder := New(db, slog.Default())
	write := func(user int, ip string, at time.Time) {
		t.Helper()
		if e := recorder.Write(ctx, Snapshot{ID: jobs.UUID(), UserID: users[user], DeviceID: devices[user], ItemID: "123456", Type: "movie", Name: "H2a 合成条目", State: "playing", IP: ip, At: at}); e != nil {
			t.Fatal(e)
		}
	}
	now := time.Now()
	write(0, "223.5.5.5", now.Add(-30*24*time.Hour))
	write(0, "223.5.5.5", now.Add(-time.Hour))
	write(0, "223.5.5.5", now.Add(-2*time.Hour))
	write(0, "240e:3b7:3272::1", now.Add(-time.Minute))
	write(1, "223.5.5.5", now.Add(-3*time.Hour))
	f := Filter{UserID: users[0], Days: 7, Limit: 25}
	out, e := (Repository{DB: db}).IPReport(ctx, f, reportLocation)
	if e != nil {
		t.Fatal(e)
	}
	if out.Total != 2 || out.Summary.ActiveUsers != 1 {
		t.Fatal(out)
	}
	for _, row := range out.Items {
		if len(row.Accounts) != 1 || len(row.Clients) != 1 || row.Clients[0].Client != "SenPlayer" {
			t.Fatal(row)
		}
		if row.IP == "223.5.5.5" && (row.Count != 2 || row.NewIP || row.FirstSeenAt.After(now.Add(-20*24*time.Hour))) {
			t.Fatal(row)
		}
		if row.IP == "240e:3b7:3272::1" && !row.DifferentRegion {
			t.Fatal(row)
		}
	}
	f.Query = "深圳"
	out, e = (Repository{DB: db}).IPReport(ctx, f, reportLocation)
	if e != nil || out.Total != 1 || !out.Items[0].DifferentRegion {
		t.Fatal(e, out)
	}
}

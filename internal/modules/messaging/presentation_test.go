package messaging

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestNoticeReadProjection(t *testing.T) {
	for _, test := range []struct{ name, kind, payload, title, body, wantTitle, wantBody string }{
		{"memory measured", "ops.alert.active", `{"metric":"memoryPercent","value":92,"threshold":90}`, "系统运维告警", "message: raw", "系统运维告警", "内存使用率92%，超过阈值90%"},
		{"legacy missing value", "ops.alert.active", `{}`, "系统运维告警", "message：宿主内存使用率过高；指标 memoryPercent，阈值 90.00", "系统运维告警", "内存使用率触发告警（阈值 90%）"},
		{"recovered", "ops.alert.recovered", `{}`, "系统告警恢复", "message: 指标 memoryPercent，阈值 90.00", "系统告警恢复", "内存使用率告警已解除（阈值 90%）"},
		{"disk unit", "ops.alert.active", `{"metric":"diskFreeGB","value":12,"threshold":20}`, "空间告警", "", "空间告警", "剩余空间12 GiB，低于阈值20 GiB"},
		{"exact threshold", "ops.alert.active", `{"metric":"memoryPercent","value":90,"threshold":90}`, "内存告警", "", "内存告警", "内存使用率90%，达到阈值90%"},
		{"neutral history", "library.refresh_requested", `{}`, "Emby 媒体库刷新已请求", "Emby 媒体库刷新已请求\nassetId: 85091f83-1111-4111-8111-111111111111", "媒体库更新已请求", ""},
		{"neutral visible", "library.visible", `{}`, "媒体已在Emby库中可见（待客户端播放验收）", "message：海边的故事\noperationId: internal-id\n46b1b438-5110-4000-8000-111111111111", "媒体库已更新（待播放器验证）", "海边的故事"},
		{"zero is a value", "ops.alert.active", `{"metric":"diskFreeGB","value":0,"threshold":20}`, "空间告警", "", "空间告警", "剩余空间0 GiB，低于阈值20 GiB"},
	} {
		t.Run(test.name, func(t *testing.T) {
			title, body := presentNotice(test.kind, []byte(test.payload), test.title, test.body)
			if title != test.wantTitle || body != test.wantBody {
				t.Fatalf("got %q / %q", title, body)
			}
			if strings.Contains(body, "assetId") || noticeUUID.MatchString(body) {
				t.Fatal(body)
			}
		})
	}
}

func TestReadableNoticeFrontendFixture(t *testing.T) {
	var fixture struct {
		Input struct {
			Kind        string
			Payload     json.RawMessage
			Title, Body string
		}
		Expected struct{ Title, Body string }
	}
	raw, err := os.ReadFile("testdata/notice-display.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	title, body := presentNotice(fixture.Input.Kind, fixture.Input.Payload, fixture.Input.Title, fixture.Input.Body)
	if title != fixture.Expected.Title || body != fixture.Expected.Body {
		t.Fatal(title, body)
	}
}

func TestLegacyWorkerNoticeIsReadProjectionOnly(t *testing.T) {
	raw := "临时库恢复、表摘要、密文和迁移幂等已验证；未连接业务Worker"
	_, body := presentNotice("ops.drill.verified", nil, "恢复演练通过", raw)
	if body != "临时库恢复、表摘要、密文和迁移幂等已验证；后台执行器未就绪" || !strings.Contains(raw, "Worker") {
		t.Fatal(body)
	}
}

func TestReadableInterruptedDeliveryError(t *testing.T) {
	if got := cleanNoticeText("Worker stopped before a confirmed send receipt; manual verification required"); got != "通知结果尚未确认，后台执行器已停止，请先人工核对" {
		t.Fatal(got)
	}
}

func TestWorkerDisplayMappingPreservesIdentifiersAndCase(t *testing.T) {
	for _, tt := range []struct{ input, want string }{
		{"plugin-worker", "plugin-worker"},
		{"worker WorkER myWorker WorkerPool", "worker WorkER myWorker WorkerPool"},
		{"Worker unavailable", "后台执行器 unavailable"},
		{"未连接业务 Worker", "后台执行器未就绪"},
		{"未连接业务Worker", "后台执行器未就绪"},
	} {
		if got := cleanNoticeText(tt.input); got != tt.want {
			t.Errorf("%q: got %q, want %q", tt.input, got, tt.want)
		}
	}
}

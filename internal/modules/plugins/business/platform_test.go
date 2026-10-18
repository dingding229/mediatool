package business

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"media-tool/internal/modules/settings"
	"media-tool/internal/platform/pluginactor"
	"reflect"
	"strings"
	"testing"
)

func TestPlatformRedLinesNoMethodsOrCredentialDTOs(t *testing.T) {
	for _, name := range []string{"integrations.get", "integrations.default", "integrations.reveal", "integrations.authorize115", "maintenance.backup", "maintenance.restore", "maintenance.dump", "maintenance.registerRelease", "jobs.create", "jobs.claim", "jobs.finish", "cache.clear", "cache.flush", "networkproxy.save", "settings.dump", "system.environ", "system.process"} {
		if _, ok := b.Lookup(name); ok {
			t.Fatal("forbidden", name)
		}
		_, e := (AcquisitionHost{}).Call(context.Background(), contracts.HostCall{Method: name, Args: []byte(`{}`)}, contracts.Permission{}, nil)
		mediaCode(t, e, contracts.Unsupported)
	}
	var inspect func(reflect.Type)
	inspect = func(typ reflect.Type) {
		for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct || typ.PkgPath() == "time" {
			return
		}
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			tag := strings.ToLower(f.Tag.Get("json"))
			for _, bad := range []string{"password", "credentials", "ciphertext", "secret", "header", "url", "environment", "payload", "body"} {
				if strings.Contains(tag, bad) {
					t.Fatal(typ.Name(), tag)
				}
			}
			inspect(f.Type)
		}
	}
	for _, v := range []any{b.Connection{}, b.ConnectionCheck{}, b.OwnedJob{}, b.AlertSummary{}, b.ProxySummary{}, b.HTTPResult{}, b.HealthSummary{}, b.DeliverySummary{}} {
		inspect(reflect.TypeOf(v))
	}
	for _, key := range []string{"access", "trustedProxyCidrs", "strmApiKey", "masterKey", "open115", "credentials", "embyUserId"} {
		if settings.PluginKey(key) {
			t.Fatal("unsafe setting", key)
		}
	}
}
func TestPlatformActorRequiredAndUnsupportedBackend(t *testing.T) {
	h := AcquisitionHost{}
	for _, method := range []string{"settings.get", "jobs.list", "integrations.list", "cache.list", "notifications.deliveries", "maintenance.alerts", "networkproxy.status", "network.http"} {
		spec, _ := b.Lookup(method)
		call := contracts.HostCall{Method: method, Args: json.RawMessage(`{}`)}
		_, e := h.Call(context.Background(), call, contracts.Permission{Scope: spec.Scope}, nil)
		mediaCode(t, e, contracts.PermissionDenied)
		_, e = h.Call(pluginactor.With(context.Background(), "fixture", spec.Scope, nil), call, contracts.Permission{Scope: spec.Scope}, nil)
		mediaCode(t, e, contracts.Unsupported)
	}
}

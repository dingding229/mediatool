package config

import (
	"strings"
	"testing"
	"time"
)

func lookup(env map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := env[k]; return v, ok }
}
func validEnv() map[string]string {
	return map[string]string{"SESSION_SECRET_KEY": strings.Repeat("a", 64), "STRM_API_KEY": strings.Repeat("b", 64)}
}
func TestDefaults(t *testing.T) {
	c, err := FromLookup(lookup(validEnv()))
	if err != nil {
		t.Fatal(err)
	}
	if c.Address != "127.0.0.1:8080" || len(c.STRMAllowedRoots) != 1 {
		t.Fatal(c)
	}
}
func TestRejectUnsafeConfig(t *testing.T) {
	for _, tc := range []struct{ k, v string }{{"APP_ENV", "staging"}, {"STRM_API_KEY", strings.Repeat("a", 64)}, {"APP_PUBLIC_URL", "https://user:pass@example.com"}, {"APP_PUBLIC_URL", "https://example.com/base"}, {"STRM_ALLOWED_ROOTS", "/115/../secret"}, {"WORKER_LEASE", "0s"}, {"EMBY_PROXY_ENABLED", "true"}} {
		t.Run(tc.k+tc.v, func(t *testing.T) {
			env := validEnv()
			env[tc.k] = tc.v
			if _, err := FromLookup(lookup(env)); err == nil {
				t.Fatal("accepted unsafe configuration")
			}
		})
	}
}
func TestProductionRequiresHTTPS(t *testing.T) {
	env := validEnv()
	env["APP_ENV"] = "production"
	if _, err := FromLookup(lookup(env)); err == nil {
		t.Fatal("http accepted")
	}
	env["APP_PUBLIC_URL"] = "https://media.example.com"
	if _, err := FromLookup(lookup(env)); err != nil {
		t.Fatal(err)
	}
}

func TestM0ConfigurationValidation(t *testing.T) {
	for _, tc := range []struct{ k, v string }{{"CLOUDDRIVE_ENDPOINT", "http://user:password@host:19798"}, {"CLOUDDRIVE_ENDPOINT", "https://host/path"}, {"CLOUDDRIVE_ENDPOINT", "http://localhost:19798"}, {"PLAYBACK_ALLOWED_HOSTS", "https://evil.example.com"}, {"EMBY_302_ENABLED", "true"}, {"CLOUDDRIVE_REQUEST_TIMEOUT", "1ms"}, {"EMBY_STRM_RESOLVER_ORIGINS", "https://example.com/path"}} {
		env := validEnv()
		env[tc.k] = tc.v
		if _, err := FromLookup(lookup(env)); err == nil {
			t.Fatal("accepted invalid config", tc.k)
		}
	}
	env := validEnv()
	env["CLOUDDRIVE_ENDPOINT"] = "http://127.0.0.1:19798"
	env["CLOUDDRIVE_USERNAME"] = "user"
	env["CLOUDDRIVE_PASSWORD"] = " password "
	c, err := FromLookup(lookup(env))
	if err != nil || c.CloudDrivePassword != " password " || !c.STRMProxyFallback {
		t.Fatal(err)
	}
}

func TestPublicGatewayOriginMustNotContainCredentialsOrPath(t *testing.T) {
	for _, raw := range []string{"https://user:secret@gateway.example", "https://gateway.example/emby", "https://gateway.example?token=private", "http://gateway.example"} {
		_, err := FromLookup(func(k string) (string, bool) {
			switch k {
			case "SESSION_SECRET_KEY":
				return strings.Repeat("a", 64), true
			case "STRM_API_KEY":
				return strings.Repeat("b", 64), true
			case "APP_ENV":
				return "production", true
			case "APP_PUBLIC_URL":
				return "https://media.example", true
			case "EMBY_PUBLIC_URL":
				return raw, true
			}
			return "", false
		})
		if err == nil {
			t.Fatal("unsafe public gateway origin accepted")
		}
	}
}

func TestDirectPlayOnlySwitchIsBooleanAndDefaultsCompatible(t *testing.T) {
	env := validEnv()
	c, err := FromLookup(lookup(env))
	if err != nil || c.EmbyDirectPlayOnly {
		t.Fatal("compatibility default changed", err)
	}
	env["EMBY_DIRECT_PLAY_ONLY"] = "true"
	c, err = FromLookup(lookup(env))
	if err != nil || !c.EmbyDirectPlayOnly {
		t.Fatal("direct-only switch missing", err)
	}
	env["EMBY_DIRECT_PLAY_ONLY"] = "maybe"
	if _, err = FromLookup(lookup(env)); err == nil {
		t.Fatal("invalid direct-only switch accepted")
	}
}

func TestPlaybackResolveBudgetIsBoundedAndIndependentOfCloudScanTimeout(t *testing.T) {
	env := validEnv()
	c, err := FromLookup(lookup(env))
	if err != nil || c.PlaybackResolveTimeout != 30*time.Second || c.CloudDriveRequestTimeout != 10*time.Second {
		t.Fatal("aggregate playback default or scanning budget changed", err)
	}
	for _, value := range []string{"5s", "30s", "1m"} {
		env["PLAYBACK_RESOLVE_TIMEOUT"] = value
		if _, err := FromLookup(lookup(env)); err != nil {
			t.Fatal(value, err)
		}
	}
	for _, value := range []string{"", "0s", "4s", "1m1s", "-1s", "nonsense"} {
		env["PLAYBACK_RESOLVE_TIMEOUT"] = value
		if _, err := FromLookup(lookup(env)); err == nil {
			t.Fatal("unsafe resolution budget accepted", value)
		}
	}
}

func TestNativeStillStorageConfiguration(t *testing.T) {
	c, err := FromLookup(lookup(validEnv()))
	if err != nil || c.NativeStillsFrameTimeout != 45*time.Second || c.NativeStillsMaxBytes != 2<<30 || c.NativeStillsRoot != "/data/native-media/stills" {
		t.Fatal(c.NativeStillsRoot, c.NativeStillsMaxBytes, err)
	}
	for _, kv := range [][2]string{{"NATIVE_STILLS_MAX_BYTES", "-1"}, {"NATIVE_STILLS_MAX_BYTES", "1099511627777"}, {"NATIVE_STILLS_ROOT", "relative/native-media/stills"}, {"NATIVE_STILLS_ROOT", "/data/../native-media/stills"}} {
		env := validEnv()
		env[kv[0]] = kv[1]
		if _, err := FromLookup(lookup(env)); err == nil {
			t.Fatal("unsafe still configuration", kv[0])
		}
	}
}

func TestPluginTaskBudget(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int
		valid bool
	}{
		{"", 256, true}, {"32", 32, true}, {"4096", 4096, true}, {"0", 0, false}, {"31", 0, false}, {"4097", 0, false}, {"bad", 0, false},
	} {
		env := validEnv()
		if tc.value != "" {
			env["PLUGIN_MAX_TASKS"] = tc.value
		}
		c, e := FromLookup(lookup(env))
		if (e == nil) != tc.valid || (tc.valid && c.PluginMaxTasks != tc.want) {
			t.Fatalf("%q: tasks=%d err=%v", tc.value, c.PluginMaxTasks, e)
		}
	}
}

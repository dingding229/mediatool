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
	return map[string]string{"APP_PUBLIC_URL": "https://media.example.test", "SESSION_SECRET_KEY": strings.Repeat("a", 64), "STRM_API_KEY": strings.Repeat("b", 64)}
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
	for _, tc := range []struct{ k, v string }{{"APP_ENV", "staging"}, {"STRM_API_KEY", strings.Repeat("a", 64)}, {"APP_PUBLIC_URL", "https://user:pass@example.com"}, {"APP_PUBLIC_URL", "https://example.com/base"}, {"STRM_ALLOWED_ROOTS", "/115/../secret"}, {"WORKER_LEASE", "0s"}} {
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
	env["APP_PUBLIC_URL"] = "http://localhost:8080"
	if _, err := FromLookup(lookup(env)); err == nil {
		t.Fatal("http accepted")
	}
	env["APP_PUBLIC_URL"] = "https://media.example.com"
	if _, err := FromLookup(lookup(env)); err != nil {
		t.Fatal(err)
	}
}

func TestM0ConfigurationValidation(t *testing.T) {
	for _, tc := range []struct{ k, v string }{{"CLOUDDRIVE_ENDPOINT", "http://user:password@host:19798"}, {"CLOUDDRIVE_ENDPOINT", "https://host/path"}, {"CLOUDDRIVE_ENDPOINT", "http://localhost:19798"}, {"PLAYBACK_ALLOWED_HOSTS", "https://evil.example.com"}, {"CLOUDDRIVE_REQUEST_TIMEOUT", "1ms"}} {
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
	if err != nil || c.CloudDrivePassword != " password " || c.STRMProxyFallback || c.STRMForceProxy {
		t.Fatal(err)
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

func TestRemovedProxyFlagsCannotEnableMediaRelay(t *testing.T) {
	for _, key := range []string{"STRM_PROXY_FALLBACK", "STRM_FORCE_PROXY"} {
		env := validEnv()
		env[key] = "true"
		c, e := FromLookup(lookup(env))
		if e != nil || c.STRMForceProxy || c.STRMProxyFallback {
			t.Fatal(key, c, e)
		}
	}
}

func TestDevelopmentRequiresExplicitDeclaration(t *testing.T) {
	for _, tc := range []struct {
		name, value  string
		present, dev bool
	}{
		{"unset", "", false, false}, {"blank", " ", true, false},
		{"production", "production", true, false}, {"explicit-development", "development", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := validEnv()
			if tc.present {
				env["APP_ENV"] = tc.value
			}
			c, err := FromLookup(lookup(env))
			if err != nil || c.Development() != tc.dev {
				t.Fatalf("mode=%q dev=%v error=%v", c.Environment, c.Development(), err)
			}
			if !tc.dev && c.RuntimeEnvironment() != "production" {
				t.Fatal("implicit development")
			}
		})
	}
	if (Config{}).Development() || (Config{}).RuntimeEnvironment() != "production" {
		t.Fatal("zero config is not production-safe")
	}
	for _, envValue := range []string{"", "production"} {
		env := validEnv()
		env["APP_ENV"] = envValue
		env["APP_PUBLIC_URL"] = "http://insecure.fixture.invalid"
		if _, err := FromLookup(lookup(env)); err == nil {
			t.Fatal("relaxed production HTTPS policy")
		}
	}
}

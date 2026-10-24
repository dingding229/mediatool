package onlineupdate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// Each required class is also exercised with a real derived Docker image in
// the full helper entrypoint; extra/future keys below have strict unit coverage.
var startupImageCases = []struct{ field, instruction string }{
	{"Entrypoint", `ENTRYPOINT ["/bin/true"]`},
	{"Cmd", `CMD ["--fixture-changed"]`},
	{"Env", `ENV STARTUP_GUARD_FIXTURE=changed`},
	{"Healthcheck", `HEALTHCHECK CMD true`},
	{"User", `USER 1000:1000`},
	{"WorkingDir", `WORKDIR /fixture-startup-change`},
	{"ExposedPorts", `EXPOSE 18099`},
	{"Volumes", `VOLUME ["/fixture-startup-volume"]`},
	{"StopSignal", `STOPSIGNAL SIGUSR1`},
	{"Shell", `SHELL ["/bin/sh", "-e", "-c"]`},
	{"OnBuild", `ONBUILD RUN true`},
}

func TestImageStartupConfigEveryFieldIsCompared(t *testing.T) {
	base := map[string]any{"Entrypoint": []any{"/app/media-tool"}, "Cmd": []any{"-role", "all"}, "Env": []any{"PATH=/bin", "STABLE=1"}, "Healthcheck": map[string]any{"Test": []any{"CMD", "true"}}, "User": "65532:65532", "WorkingDir": "/app", "ExposedPorts": map[string]any{"8080/tcp": map[string]any{}}, "Volumes": map[string]any{"/data": map[string]any{}}, "StopSignal": "SIGTERM", "Labels": map[string]any{"version": "0.28.4"}}
	fields := []string{}
	for _, c := range startupImageCases {
		fields = append(fields, c.field)
	}
	fields = append(fields, "StopTimeout", "ArgsEscaped", "AttachStdin", "AttachStdout", "AttachStderr", "Tty", "OpenStdin", "StdinOnce", "Hostname", "Domainname", "FutureStartupOption")
	clone := func() map[string]any {
		b, _ := json.Marshal(base)
		var v map[string]any
		_ = json.Unmarshal(b, &v)
		return v
	}
	if !sameImageStartupConfig(base, clone()) {
		t.Fatal("same config denied")
	}
	target := clone()
	target["Labels"] = map[string]any{"version": "0.28.5", "commit": "public-metadata"}
	if !sameImageStartupConfig(base, target) {
		t.Fatal("documented metadata-only label change denied")
	}
	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			v := clone()
			v[field] = []any{"changed-fixture"}
			if sameImageStartupConfig(base, v) || sameImageStartupConfig(v, base) {
				t.Fatal("startup key ignored", field)
			}
		})
	}
	for _, field := range []string{"Cmd", "Env", "User"} {
		v := clone()
		delete(v, field)
		if sameImageStartupConfig(base, v) {
			t.Fatal("missing field ignored", field)
		}
	}
	target = clone()
	target["Env"] = []any{"PATH=/bin", "STABLE=1", "VERSION=0.28.5"}
	if sameImageStartupConfig(base, target) {
		t.Fatal("version ENV was silently excluded")
	}
	target = clone()
	target["FutureStartupOption"] = nil
	if sameImageStartupConfig(base, target) {
		t.Fatal("new null field was silently excluded")
	}
	if sameImageStartupConfig(nil, map[string]any{}) {
		t.Fatal("missing image config trusted")
	}
}

func TestImageStartupGuardReadsImmutableImagesAndDoesNotWrite(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "same", true: "changed"}[changed], func(t *testing.T) {
			d := testDocker()
			paths := []string{}
			d.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" {
					t.Fatal("guard wrote Docker")
				}
				paths = append(paths, r.URL.Path)
				config := map[string]any{"Env": []any{"STABLE=1"}}
				id := d.self.Image
				if strings.Contains(r.URL.Path, d.target) {
					id = "sha256:target"
					if changed {
						config["Env"] = []any{"STABLE=2"}
					}
				}
				raw, _ := json.Marshal(map[string]any{"Id": id, "Config": config})
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw))), Header: make(http.Header)}, nil
			})}
			e := d.imageStartupPreflight(context.Background())
			if changed && !errors.Is(e, ErrImageConfigChanged) || !changed && e != nil {
				t.Fatal(e)
			}
			if len(paths) != 2 || paths[0] != apiVersion+"/images/"+d.self.Image+"/json" || paths[1] != apiVersion+"/images/"+d.target+"/json" {
				t.Fatal(paths)
			}
			if fmtString(d.imageConfig["Env"].([]any)[0]) != map[bool]string{false: "STABLE=1", true: "STABLE=2"}[changed] {
				t.Fatal("target cache was replaced by old image")
			}
		})
	}
}

func TestDockerfileRuntimeDoesNotEmbedVersionInEnv(t *testing.T) {
	raw, e := os.ReadFile("../../../deploy/Dockerfile")
	if e != nil {
		t.Fatal(e)
	}
	runtime := strings.Split(string(raw), "AS runtime")
	if len(runtime) != 2 {
		t.Fatal("runtime stage not found")
	}
	// Runtime has no ENV instructions at all; build metadata belongs in labels /
	// ldflags. Base-image ENV remains subject to the exact Config comparison.
	for _, line := range strings.Split(runtime[1], "\n") {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(line)), "ENV ") {
			t.Fatal("runtime ENV added; audit image startup stability", line)
		}
	}
}

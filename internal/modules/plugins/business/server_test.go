package business

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"go/parser"
	"go/token"
	"media-tool/internal/platform/pluginactor"
	"os"
	"reflect"
	"strings"
	"testing"
)

var forbiddenServerMethods = []string{"playback.resolve", "playback.streamURLs", "playback.activate", "playback.playing", "playback.stream", "mediaserver.playback", "mediaserver.image", "emby.gateway", "emby.proxy", "emby.managedLink", "emby.metadataHeaders", "mediausers.login", "mediausers.session", "mediausers.passwordHash", "mediausers.token", "playeraccess.events", "playeraccess.policy.update"}

func TestPlaybackRedLineNoRPCRegistrationOrSDKURLFields(t *testing.T) {
	h := AcquisitionHost{}
	for _, name := range forbiddenServerMethods {
		if _, ok := b.Lookup(name); ok {
			t.Fatal("forbidden method", name)
		}
		_, e := h.Call(context.Background(), contracts.HostCall{Method: name, Args: []byte(`{}`)}, contracts.Permission{}, nil)
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
			tag := strings.ToLower(strings.Split(f.Tag.Get("json"), ",")[0])
			for _, word := range []string{"url", "path", "password", "token", "clientip", "useragent", "headers", "pickcode"} {
				if strings.Contains(tag, word) {
					t.Fatal("forbidden field", typ.Name(), tag)
				}
			}
			inspect(f.Type)
		}
	}
	for _, v := range []any{b.ServerItem{}, b.ServerLibrary{}, b.ServerUser{}, b.ServerDevice{}, b.PlaybackSession{}, b.PlaybackCounts{}, b.AccessCounts{}, b.NativeMetadata{}, b.ServerConfiguration{}} {
		inspect(reflect.TypeOf(v))
	}
	// Core Host code must not introduce another execution/link acquisition path.
	for _, name := range []string{"server.go", "processing.go"} {
		raw, e := os.ReadFile(name)
		if e != nil {
			t.Fatal(e)
		}
		f, e := parser.ParseFile(token.NewFileSet(), name, raw, parser.ImportsOnly)
		if e != nil {
			t.Fatal(e)
		}
		for _, im := range f.Imports {
			if strings.Contains(im.Path.Value, "/ffx") || strings.Contains(im.Path.Value, "/linksource") {
				t.Fatal(im.Path.Value)
			}
		}
		for _, marker := range []string{".Playback(", ".Resolve(", ".Image(", ".Activate(", ".ManagedLink(", ".MetadataHeaders("} {
			if strings.Contains(string(raw), marker) {
				t.Fatal(name, marker)
			}
		}
	}
}
func TestServerActorCannotComeFromArguments(t *testing.T) {
	h := AcquisitionHost{}
	for _, name := range []string{"catalog.list", "mediaserver.items", "mediausers.get"} {
		spec, _ := b.Lookup(name)
		_, e := h.Call(context.Background(), contracts.HostCall{Method: name, Args: []byte(`{"userId":"other","visibleLibraries":["all"]}`)}, contracts.Permission{Scope: spec.Scope}, nil)
		mediaCode(t, e, contracts.PermissionDenied)
	}
	ctx := pluginactor.With(context.Background(), "host-instance", "mediaserver.read", []string{"g:movie"})
	if _, e := pluginactor.Require(ctx, "mediaserver.users.write"); e == nil {
		t.Fatal("scope elevation")
	}
	for _, name := range []string{"catalog.home", "catalog.discover", "catalog.nextUp", "catalog.userData"} {
		spec, _ := b.Lookup(name)
		_, e := h.Call(ctx, contracts.HostCall{Method: name, Args: json.RawMessage(`{}`)}, contracts.Permission{Scope: spec.Scope}, nil)
		mediaCode(t, e, contracts.Unsupported)
	}
}

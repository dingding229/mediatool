package v2

import (
	"context"
	"errors"
	"flag"
	"github.com/dingding229/mediatool-plugin/registry"
	"github.com/dingding229/mediatool-plugin/sdk"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/dingding229/mediatool-plugin/sdk/transport"
	"io"
	"log/slog"
	"media-tool/internal/modules/plugins/v2/sandbox"
	"os"
	"path/filepath"
)

// Worker is called BEFORE app configuration, logging store, migration or DB setup.
func Worker(args []string) error {
	flags := flag.NewFlagSet("plugin-worker", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	plugin := flags.String("plugin", "", "")
	instance := flags.String("instance", "", "")
	fp := flags.String("fingerprint", "", "")
	if len(args) != 6 || flags.Parse(args) != nil || flags.NArg() != 0 || !validUUID(*instance) {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for i := 0; i < len(args); i += 2 {
		if seen[args[i]] {
			return ErrInvalid
		}
		seen[args[i]] = true
	}
	d, e := definition(*plugin)
	if e != nil || d.Fingerprint != *fp {
		return ErrInvalid
	}
	cwd, e := os.Getwd()
	if e != nil || filepath.Base(cwd) != *instance {
		return ErrInvalid
	}
	if e = sandbox.Limits(); e != nil {
		return ErrUnavailable
	}
	ok, e := sandbox.Landlock(filepath.Join(cwd, "data"), filepath.Join(cwd, "tmp"), nil)
	if e != nil {
		return ErrUnavailable
	}
	if !ok {
		sandbox.WarnUnavailable(slog.New(slog.NewTextHandler(os.Stderr, nil)), *instance)
	}
	entry, e := registry.Get(*plugin)
	if e != nil {
		return ErrUnavailable
	}
	peer, e := transport.New(&transport.Stream{Reader: os.Stdin, Writer: os.Stdout}, transport.Options{Prefix: "p", MaxHandlers: 1})
	if e != nil {
		return ErrUnavailable
	}
	run, e := sdk.NewRuntime(peer, entry.Manifest, entry.Factory, sdk.Identity{InstanceID: *instance, DefinitionFingerprint: *fp, CatalogBuild: BuildID()})
	if e != nil {
		return ErrUnavailable
	}
	e = run.Serve(context.Background())
	var rpc *contracts.Error
	if errors.As(e, &rpc) && (rpc.Code == contracts.Unavailable || rpc.Code == contracts.Cancelled) {
		return nil
	}
	return e
}

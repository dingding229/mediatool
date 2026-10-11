package clouddrive2

import (
	"context"
	"errors"
	"google.golang.org/grpc/connectivity"
	"media-tool/internal/modules/storage"
	"testing"
)

func TestRetireDrainsActiveOperations(t *testing.T) {
	a, err := New(Options{Endpoint: "http://127.0.0.1:1", Token: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	ctx, release, err := a.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	a.Retire()
	if a.conn.GetState() == connectivity.Shutdown {
		t.Fatal("active operation closed")
	}
	if _, _, err = a.acquire(context.Background()); !errors.Is(err, storage.ErrUnavailable) {
		t.Fatal("retired accepted work")
	}
	_, nested, err := a.acquire(ctx)
	if err != nil {
		t.Fatal("nested active lookup refused")
	}
	nested()
	release()
	if a.conn.GetState() != connectivity.Shutdown {
		t.Fatal("idle retired connection remained open")
	}
}

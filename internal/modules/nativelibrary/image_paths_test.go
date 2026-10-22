package nativelibrary

import (
	"reflect"
	"testing"
)

func TestF4FixedImagePathsAndPersonNamespace(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"/logo.svg", "/logo.png"}, {"/actor.jpg", "/actor.jpg"}, {"/logo.png", "/logo.png"}, {"https://evil.invalid/a.png", ""}, {"//evil.invalid/a.png", ""}, {"/a/b.jpg", ""}, {"/../a.jpg", ""}, {"/%2e%2e.png", ""}, {"/a.jpg?token=x", ""}, {"/a.svg/../../a.png", ""}, {"/a.png#x", ""}, {"/a.txt", ""},
	} {
		if got := ImagePath(tc.in); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	if got := ImagePaths("/preferred.svg", []string{"/second.png", "/preferred.png", "/../../x.jpg"}); !reflect.DeepEqual(got, []string{"/preferred.png", "/second.png"}) {
		t.Fatal(got)
	}
	for _, id := range []int64{1, 1392137, 9223372036854775807} {
		p := PersonID(id)
		if n, ok := ParsePersonID(p); !ok || n != id || len(p) != 32 {
			t.Fatal(id, p, n, ok)
		}
	}
	for _, p := range []string{"1392137", "100001", "person:1", "706572736f6e00000000000000000000", "706572736f6e00008000000000000000"} {
		if _, ok := ParsePersonID(p); ok {
			t.Fatal(p)
		}
	}
	if ImageTag(ImagePath("/old.svg")) == ImageTag(ImagePath("/new.svg")) {
		t.Fatal("source change must invalidate tag")
	}
}

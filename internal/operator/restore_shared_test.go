package operator

import (
	"context"
	"strings"
	"testing"
)

func TestDataRestoreDelegatesToSharedAppExecutorWithoutStoppingOrSwapping(t *testing.T) {
	calls := []string{}
	o := &owner{ctx: context.Background(), control: []string{"app", "ops"}, command: func(args []string) (string, error) {
		s := strings.Join(args, " ")
		calls = append(calls, s)
		if strings.Contains(s, "restore-request") {
			return `{"state":"completed","message":"restore done"}`, nil
		}
		return `{"id":"11111111-1111-4111-8111-111111111111","kind":"restore","state":"approved"}`, nil
	}}
	out := o.execute("11111111-1111-4111-8111-111111111111")
	if o.err != nil || out["state"] != "completed" {
		t.Fatal(out, o.err)
	}
	if len(calls) != 2 || !strings.Contains(calls[1], "restore-request") {
		t.Fatal(calls)
	}
	for _, v := range calls {
		for _, bad := range []string{" stop ", "prepare", "pg_terminate_backend", "ALTER DATABASE", "docker cp"} {
			if strings.Contains(v, bad) {
				t.Fatal("host restore path used", v)
			}
		}
	}
}

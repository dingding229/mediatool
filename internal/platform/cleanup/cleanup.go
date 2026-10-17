// Package cleanup makes non-actionable, post-result resource release explicit.
// Never use Ignore for persistence, parsing, primary I/O, or state transitions.
package cleanup

import (
	"context"
	"io"
	"log/slog"
	"time"
)

// Ignore is permitted only with a call-site rationale. It intentionally does
// not log: teardown can run while the logging store mutex is held, and logging
// here could deadlock or disclose an upstream URL from an error. The primary
// operation must already have checked its own result before using this helper.
func Ignore(err error, rationale string) {
	if rationale == "" {
		panic("cleanup requires an explicit rationale")
	}
	if err != nil {
		return
	} // Deliberate, documented non-actionable teardown outcome.
}
func Close(resource io.Closer, rationale string) { Ignore(resource.Close(), rationale) }

type ContextCloser interface{ Close(context.Context) error }

func CloseContext(ctx context.Context, resource ContextCloser, rationale string) {
	Ignore(resource.Close(ctx), rationale)
}

type Transaction interface{ Rollback(context.Context) error }

// Rollback is abort-only and deferred: the original operation/Commit result is
// authoritative. pgx releases/discards a failed rollback connection. A detached
// bounded context still attempts cleanup when the request itself was cancelled.
func Rollback(parent context.Context, tx Transaction) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 2*time.Second)
	defer cancel()
	Ignore(tx.Rollback(ctx), "Deferred abort only; original operation or Commit result is authoritative; pgx discards failed rollback connections")
}
func Do(action func() error, rationale string) { Ignore(action(), rationale) }

// Report is for callbacks with no error return (e.g. post-failure bookkeeping).
// Only a fixed operation name is logged, never error text containing credentials.
func Report(err error, operation string) {
	if err != nil {
		slog.Warn("Background bookkeeping failed", "operation", operation)
	}
}

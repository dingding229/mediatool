// Package fixtures contains helpers used only by isolated integration tests.
package fixtures

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"media-tool/internal/modules/accounts"
)

func Initialization(t testing.TB, ctx context.Context, repo accounts.Repository) string {
	t.Helper()
	var name string
	if err := repo.DB.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal("isolated media_tool_test required")
	}
	var stdout bytes.Buffer
	if err := repo.PrintInitialization(ctx, &stdout); err != nil {
		t.Fatal(err)
	}
	line, found := strings.CutPrefix(stdout.String(), "[ADMIN INITIALIZATION] Token: ")
	token, _, ended := strings.Cut(line, " | ")
	if !found || !ended || !accounts.ValidInitializationToken(token) {
		t.Fatal("startup did not issue a valid initialization token")
	}
	return token
}

func Register(t testing.TB, ctx context.Context, repo accounts.Repository, username, password, session string, meta ...accounts.SessionMetadata) (accounts.Account, error) {
	t.Helper()
	return repo.Register(ctx, username, password, session, Initialization(t, ctx, repo), meta...)
}

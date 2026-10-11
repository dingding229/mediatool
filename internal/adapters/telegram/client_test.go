package telegram

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestDeliveryOutcomeClassificationDoesNotExposeURLToken(t *testing.T) {
	for _, tc := range []struct {
		name           string
		status         int
		body           string
		network        bool
		unknown, retry bool
	}{{"rejected", 403, `{"ok":false,"error_code":403}`, false, false, false}, {"limited", 429, `{"ok":false,"error_code":429,"parameters":{"retry_after":30}}`, false, false, true}, {"server", 503, `{"ok":false,"error_code":503}`, false, false, true}, {"timeout", 0, "", true, true, false}, {"malformed", 200, "bad json", false, true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			client := Client{Token: "FILE-PRIVATE-TOKEN", ChatID: "1", HTTP: &http.Client{Transport: fixtureTransport(func(*http.Request) (*http.Response, error) {
				if tc.network {
					return nil, errors.New("https://api.telegram.org/botFILE-PRIVATE-TOKEN/sendMessage")
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{}}, nil
			})}}
			_, _, err := client.SendText(context.Background(), "owned notification")
			classified, ok := err.(*RequestError)
			if !ok || classified.Unknown != tc.unknown || classified.Retryable != tc.retry || strings.Contains(err.Error(), client.Token) {
				t.Fatal(err)
			}
		})
	}
}

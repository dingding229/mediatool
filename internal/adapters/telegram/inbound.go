package telegram

import (
	"context"
	"encoding/json"
	"io"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"strings"
	"time"
)

// The inbound transport is separate from outbound SendText: no change to its
// retries/target/receipt semantics. No token URL or provider body is returned.
type Sender struct {
	ID    int64 `json:"id"`
	IsBot bool  `json:"is_bot"`
}
type Inbound struct {
	Date int64  `json:"date"`
	Text string `json:"text"`
	From Sender `json:"from"`
	Chat struct {
		ID int64 `json:"id"`
	} `json:"chat"`
}
type Callback struct {
	ID      string   `json:"id"`
	From    Sender   `json:"from"`
	Message *Inbound `json:"message"`
	Data    string   `json:"data"`
}
type Update struct {
	ID       int64     `json:"update_id"`
	Message  *Inbound  `json:"message"`
	Callback *Callback `json:"callback_query"`
}

func (c Client) Updates(ctx context.Context, offset int64) ([]Update, error) {
	if c.Token == "" || offset < 0 {
		return nil, ErrRequest
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	raw, _ := json.Marshal(map[string]any{"offset": offset, "timeout": 15, "limit": 10, "allowed_updates": []string{"message", "callback_query"}})
	r, e := http.NewRequestWithContext(ctx, "POST", "https://api.telegram.org/bot"+c.Token+"/getUpdates", strings.NewReader(string(raw)))
	if e != nil {
		return nil, ErrRequest
	}
	r.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 20 * time.Second}
	if c.HTTP != nil {
		client = *c.HTTP
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, e := client.Do(r)
	if e != nil {
		return nil, ErrRequest
	}
	defer deferutil.Close(response.Body, "bounded inbound body teardown; primary read handles I/O")
	body, e := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if e != nil || len(body) > 1<<20 || response.StatusCode != 200 {
		return nil, ErrRequest
	}
	var envelope Response
	var updates []Update
	if json.Unmarshal(body, &envelope) != nil || !envelope.OK || json.Unmarshal(envelope.Result, &updates) != nil || len(updates) > 10 {
		return nil, ErrRequest
	}
	for _, u := range updates {
		if u.ID < 0 || u.ID > 9007199254740991 {
			return nil, ErrRequest
		}
	}
	return updates, nil
}
func (c Client) AnswerCallback(ctx context.Context, id string) error {
	if id == "" || len(id) > 200 {
		return ErrRequest
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	client := http.Client{Timeout: 3 * time.Second}
	if c.HTTP != nil {
		client = *c.HTTP
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c.HTTP = &client
	_, e := c.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": id})
	return e
}

package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

type RequestError struct {
	Unknown   bool
	Retryable bool
	HTTPCode  int
}

func (e *RequestError) Error() string {
	return "Telegram request failed; raw URLs and tokens are hidden"
}

var ErrRequest = errors.New("Telegram request failed; check bot token, chat and connectivity")

type Client struct {
	Token, ChatID string
	HTTP          *http.Client
}
type Response struct {
	OK         bool            `json:"ok"`
	Result     json.RawMessage `json:"result"`
	Code       int             `json:"error_code"`
	Parameters struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

func (c Client) call(ctx context.Context, method string, body any) (Response, error) {
	raw, _ := json.Marshal(body)
	r, err := http.NewRequestWithContext(ctx, "POST", "https://api.telegram.org/bot"+c.Token+"/"+method, strings.NewReader(string(raw)))
	if err != nil {
		return Response{}, &RequestError{Unknown: false}
	}
	r.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	resp, err := client.Do(r)
	if err != nil {
		return Response{}, &RequestError{Unknown: true}
	}
	defer resp.Body.Close()
	var result Response
	if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&result) != nil {
		return result, &RequestError{Unknown: true, HTTPCode: resp.StatusCode}
	}
	if !result.OK {
		return result, &RequestError{Unknown: false, Retryable: resp.StatusCode >= 500 || resp.StatusCode == 429, HTTPCode: resp.StatusCode}
	}
	return result, nil
}
func (c Client) Probe(ctx context.Context) error {
	if c.Token == "" || c.ChatID == "" {
		return errors.New("configure Bot Token and Chat ID")
	}
	if _, err := c.call(ctx, "getMe", map[string]any{}); err != nil {
		return err
	}
	_, err := c.call(ctx, "getChat", map[string]string{"chat_id": c.ChatID})
	return err
}
func (c Client) SendText(ctx context.Context, text string) (string, int, error) {
	result, err := c.call(ctx, "sendMessage", map[string]any{"chat_id": c.ChatID, "text": text, "link_preview_options": map[string]bool{"is_disabled": true}})
	if err != nil {
		return "", result.Parameters.RetryAfter, err
	}
	var message struct {
		MessageID json.Number `json:"message_id"`
	}
	if json.Unmarshal(result.Result, &message) != nil {
		return "", 0, &RequestError{Unknown: true}
	}
	if message.MessageID.String() == "" || message.MessageID.String() == "0" {
		return "", 0, &RequestError{Unknown: true}
	}
	return message.MessageID.String(), 0, nil
}

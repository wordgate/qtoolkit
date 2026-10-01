package slack

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrInvalidSignature is wrapped by every VerifySignature failure.
var ErrInvalidSignature = errors.New("slack: invalid request signature")

// maxSignatureAge is how far X-Slack-Request-Timestamp may be from now, in
// either direction, before a request is rejected as a possible replay.
const maxSignatureAge = 5 * time.Minute

// RateLimitError is returned when Slack answers HTTP 429.
// RetryAfter comes from the Retry-After response header; it is 0 if the
// header is missing or malformed, so callers should apply their own minimum.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("slack: rate limited, retry after %s", e.RetryAfter)
}

// PostOptions are optional parameters for PostMessage.
type PostOptions struct {
	ThreadTS       string // non-empty: post as a reply in this thread
	ReplyBroadcast bool   // also show the thread reply in the channel
}

// callAPI calls a Slack Web API method with the configured bot token.
//
// A non-nil payload is sent as a JSON POST body; otherwise the call is a GET
// with query parameters. The response is decoded into out (if non-nil).
// HTTP 429 yields *RateLimitError; any other failure, including an HTTP 200
// whose body says "ok": false, wraps ErrAPIFailed with Slack's error code.
func callAPI(ctx context.Context, method string, query url.Values, payload any, out any) error {
	cfg := getConfig()
	if cfg.BotToken == "" {
		return ErrNoBotToken
	}

	reqURL := slackAPIBase + "/" + method
	if len(query) > 0 {
		reqURL += "?" + query.Encode()
	}

	httpMethod := http.MethodGet
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("%w: %s: %v", ErrAPIFailed, method, err)
		}
		httpMethod = http.MethodPost
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, httpMethod, reqURL, body)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrAPIFailed, method, err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.BotToken)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		// %w on both so callers can match context.Canceled / DeadlineExceeded
		return fmt.Errorf("%w: %s: %w", ErrAPIFailed, method, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		rl := &RateLimitError{}
		if secs, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && secs > 0 {
			rl.RetryAfter = time.Duration(secs) * time.Second
		}
		return rl
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrAPIFailed, method, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s: status %d", ErrAPIFailed, method, resp.StatusCode)
	}

	var status struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &status); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrAPIFailed, method, err)
	}
	if !status.OK {
		return fmt.Errorf("%w: %s: %s", ErrAPIFailed, method, status.Error)
	}

	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("%w: %s: %v", ErrAPIFailed, method, err)
		}
	}
	return nil
}

// PostMessage posts text to a channel (chat.postMessage) and returns the new
// message's ts. With opts.ThreadTS set, the message is a reply in that thread.
// Requires bot_token with the chat:write scope.
func PostMessage(ctx context.Context, channelID, text string, opts *PostOptions) (ts string, err error) {
	payload := map[string]any{
		"channel": channelID,
		"text":    truncateText(text, MaxTextLength, defaultTruncationSuffix),
	}
	if opts != nil && opts.ThreadTS != "" {
		payload["thread_ts"] = opts.ThreadTS
		if opts.ReplyBroadcast {
			payload["reply_broadcast"] = true
		}
	}

	var result struct {
		TS string `json:"ts"`
	}
	if err := callAPI(ctx, "chat.postMessage", nil, payload, &result); err != nil {
		return "", err
	}
	if result.TS == "" {
		return "", fmt.Errorf("%w: chat.postMessage: response has no ts", ErrAPIFailed)
	}
	return result.TS, nil
}

// UpdateMessage replaces the text of an existing message (chat.update).
// Requires bot_token with the chat:write scope; a bot can only update its own messages.
func UpdateMessage(ctx context.Context, channelID, ts, text string) error {
	payload := map[string]any{
		"channel": channelID,
		"ts":      ts,
		"text":    truncateText(text, MaxTextLength, defaultTruncationSuffix),
	}
	return callAPI(ctx, "chat.update", nil, payload, nil)
}

// GetPermalink returns the permalink URL of a message (chat.getPermalink).
func GetPermalink(ctx context.Context, channelID, ts string) (string, error) {
	query := url.Values{"channel": {channelID}, "message_ts": {ts}}
	var result struct {
		Permalink string `json:"permalink"`
	}
	if err := callAPI(ctx, "chat.getPermalink", query, nil, &result); err != nil {
		return "", err
	}
	if result.Permalink == "" {
		return "", fmt.Errorf("%w: chat.getPermalink: response has no permalink", ErrAPIFailed)
	}
	return result.Permalink, nil
}

// PinMessage pins a message to its channel (pins.add).
// Requires bot_token with the pins:write scope. Pinning an already pinned
// message fails with Slack's "already_pinned" error.
func PinMessage(ctx context.Context, channelID, ts string) error {
	payload := map[string]any{
		"channel":   channelID,
		"timestamp": ts,
	}
	return callAPI(ctx, "pins.add", nil, payload, nil)
}

// UserEmail returns a user's profile email (users.info).
// Requires bot_token with the users:read and users:read.email scopes.
// An unknown user yields ErrUserNotFound; a user without a visible email
// (bot users, or a token lacking users:read.email) yields an error rather
// than an empty string.
func UserEmail(ctx context.Context, userID string) (string, error) {
	var result struct {
		User struct {
			Profile struct {
				Email string `json:"email"`
			} `json:"profile"`
		} `json:"user"`
	}
	err := callAPI(ctx, "users.info", url.Values{"user": {userID}}, nil, &result)
	if err != nil {
		if errors.Is(err, ErrAPIFailed) && strings.HasSuffix(err.Error(), ": user_not_found") {
			return "", fmt.Errorf("%w: %s", ErrUserNotFound, userID)
		}
		return "", err
	}
	if result.User.Profile.Email == "" {
		return "", fmt.Errorf("%w: users.info: user %s has no profile email (bot user, or token lacks users:read.email)", ErrAPIFailed, userID)
	}
	return result.User.Profile.Email, nil
}

// VerifySignature verifies a Slack request signature (v0 scheme) for Events
// API, slash command and interactivity requests.
//
// body must be the raw request body exactly as received. The signature is
// HMAC-SHA256(signingSecret, "v0:"+timestamp+":"+body), compared in constant
// time with X-Slack-Signature. Requests whose X-Slack-Request-Timestamp
// differs from now by more than 5 minutes are rejected. An empty signingSecret
// is always rejected. Every failure wraps ErrInvalidSignature.
func VerifySignature(signingSecret string, header http.Header, body []byte, now time.Time) error {
	if signingSecret == "" {
		return fmt.Errorf("%w: signing secret not configured", ErrInvalidSignature)
	}

	timestamp := header.Get("X-Slack-Request-Timestamp")
	signature := header.Get("X-Slack-Signature")
	if timestamp == "" {
		return fmt.Errorf("%w: missing X-Slack-Request-Timestamp header", ErrInvalidSignature)
	}
	if signature == "" {
		return fmt.Errorf("%w: missing X-Slack-Signature header", ErrInvalidSignature)
	}

	sec, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: malformed timestamp", ErrInvalidSignature)
	}
	age := now.Sub(time.Unix(sec, 0))
	if age < 0 {
		age = -age
	}
	if age > maxSignatureAge {
		return fmt.Errorf("%w: timestamp outside the %s window", ErrInvalidSignature, maxSignatureAge)
	}

	hexSig, ok := strings.CutPrefix(signature, "v0=")
	if !ok {
		return fmt.Errorf("%w: unsupported signature version", ErrInvalidSignature)
	}
	got, err := hex.DecodeString(hexSig)
	if err != nil {
		return fmt.Errorf("%w: malformed signature", ErrInvalidSignature)
	}

	mac := hmac.New(sha256.New, []byte(signingSecret))
	mac.Write([]byte("v0:" + timestamp + ":"))
	mac.Write(body)
	if !hmac.Equal(got, mac.Sum(nil)) {
		return fmt.Errorf("%w: signature mismatch", ErrInvalidSignature)
	}
	return nil
}

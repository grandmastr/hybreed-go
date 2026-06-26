// Package notify sends remote push notifications via the Expo Push API.
//
// It is intentionally dependency-free (stdlib net/http + encoding/json): given a
// user id it looks up that user's registered device tokens, fans the message out
// to Expo, and prunes tokens Expo reports as no longer registered.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/grandmastr/hybreed-go/internal/store"
)

// expoPushURL is the Expo Push API endpoint that accepts a JSON array of messages.
const expoPushURL = "https://exp.host/--/api/v2/push/send"

// Service sends Expo push notifications for a user's registered devices.
type Service struct {
	q    *store.Queries
	http *http.Client
	log  *slog.Logger
}

// NewService builds the push sender with a short-timeout HTTP client.
func NewService(q *store.Queries, log *slog.Logger) *Service {
	return &Service{
		q:    q,
		http: &http.Client{Timeout: 10 * time.Second},
		log:  log,
	}
}

// message is a single Expo push message. Sound is always "default"; Data is
// optional structured payload the client app can route on.
type message struct {
	To    string         `json:"to"`
	Title string         `json:"title"`
	Body  string         `json:"body"`
	Sound string         `json:"sound"`
	Data  map[string]any `json:"data,omitempty"`
}

// pushTicket mirrors one element of the Expo response "data" array. Tickets align
// positionally with the request messages.
type pushTicket struct {
	Status  string `json:"status"`
	ID      string `json:"id"`
	Message string `json:"message"`
	Details struct {
		Error string `json:"error"`
	} `json:"details"`
}

type pushResponse struct {
	Data []pushTicket `json:"data"`
}

// Send delivers (title, body, data) to every device registered for userID and
// returns how many tokens it attempted to send to. It returns (0, nil) when the
// user has no registered devices. Tokens Expo reports as DeviceNotRegistered are
// pruned best-effort. It never panics; transport/encoding failures are wrapped.
func (s *Service) Send(ctx context.Context, userID uuid.UUID, title, body string, data map[string]any) (int, error) {
	tokens, err := s.q.ListUserPushTokens(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("list push tokens: %w", err)
	}
	if len(tokens) == 0 {
		return 0, nil
	}

	msgs := make([]message, 0, len(tokens))
	for _, t := range tokens {
		msgs = append(msgs, message{
			To:    t.Token,
			Title: title,
			Body:  body,
			Sound: "default",
			Data:  data,
		})
	}

	payload, err := json.Marshal(msgs)
	if err != nil {
		return 0, fmt.Errorf("marshal push payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, expoPushURL, bytes.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("build push request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("post to expo: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var parsed pushResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, fmt.Errorf("decode expo response: %w", err)
	}

	// Tickets align positionally with the request. Prune any token Expo reports
	// as no longer registered (best-effort: log on error, keep going).
	for i, ticket := range parsed.Data {
		if i >= len(tokens) {
			break
		}
		if ticket.Status == "error" && ticket.Details.Error == "DeviceNotRegistered" {
			token := tokens[i].Token
			if derr := s.q.DeletePushTokenByValue(ctx, token); derr != nil && s.log != nil {
				s.log.Error("prune push token", "err", derr, "token", token)
			}
		}
	}

	return len(tokens), nil
}

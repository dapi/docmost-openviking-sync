package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const maxBodySize = 1 << 20

type Event struct {
	Version     string `json:"version"`
	ID          string `json:"id"`
	Event       string `json:"event"`
	OccurredAt  string `json:"occurredAt"`
	WorkspaceID string `json:"workspaceId"`
	Data        struct {
		PageID string `json:"pageId"`
	} `json:"data"`
}

func NewHandler(secret string, trigger func()) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodySize))
		if err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if !validSignature(secret, body, r.Header.Get("X-Docmost-Signature-256")) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}

		var event Event
		if err := json.Unmarshal(body, &event); err != nil || event.Version != "1" || event.ID == "" || event.Data.PageID == "" || !supportedEvent(event.Event) {
			http.Error(w, "invalid event", http.StatusBadRequest)
			return
		}

		trigger()
		w.WriteHeader(http.StatusAccepted)
	})
}

func validSignature(secret string, body []byte, value string) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	actual, err := hex.DecodeString(strings.TrimPrefix(value, prefix))
	if err != nil {
		return false
	}
	expected := hmac.New(sha256.New, []byte(secret))
	expected.Write(body)
	return hmac.Equal(actual, expected.Sum(nil))
}

func supportedEvent(name string) bool {
	switch name {
	case "page.created", "page.updated", "page.moved", "page.deleted", "page.restored":
		return true
	default:
		return false
	}
}

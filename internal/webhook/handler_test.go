package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerAcceptsSignedEvent(t *testing.T) {
	const secret = "test-secret-with-at-least-thirty-two-characters"
	body := `{"version":"1","id":"delivery-1","event":"page.updated","occurredAt":"2026-08-29T12:00:00Z","workspaceId":"workspace-1","data":{"pageId":"page-1"}}`
	triggered := false
	handler := NewHandler(secret, func() { triggered = true })
	req := httptest.NewRequest(http.MethodPost, "/events/docmost", strings.NewReader(body))
	req.Header.Set("X-Docmost-Signature-256", sign(secret, body))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusAccepted || !triggered {
		t.Fatalf("status=%d triggered=%v body=%s", res.Code, triggered, res.Body.String())
	}
}

func TestHandlerRejectsBadSignature(t *testing.T) {
	handler := NewHandler("correct-secret", func() { t.Fatal("must not trigger") })
	req := httptest.NewRequest(http.MethodPost, "/events/docmost", strings.NewReader(`{"version":"1"}`))
	req.Header.Set("X-Docmost-Signature-256", sign("wrong-secret", `{"version":"1"}`))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", res.Code)
	}
}

func TestHandlerRejectsUnknownEvent(t *testing.T) {
	const secret = "test-secret"
	body := `{"version":"1","id":"delivery-1","event":"user.created","data":{"pageId":"page-1"}}`
	handler := NewHandler(secret, func() { t.Fatal("must not trigger") })
	req := httptest.NewRequest(http.MethodPost, "/events/docmost", strings.NewReader(body))
	req.Header.Set("X-Docmost-Signature-256", sign(secret, body))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", res.Code)
	}
}

func sign(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

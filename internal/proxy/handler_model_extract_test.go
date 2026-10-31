package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Muriel-Gasparini/antigravity-account-switcher/internal/domain"
)

// serveCountingExtracts sends one request through the handler and returns how
// many times the request was scanned for its model.
func serveCountingExtracts(t *testing.T, env *fallbackTestEnv, path, body string) (scans int32, upstreamPath string) {
	t.Helper()
	ctx := context.Background()

	now := time.Now().UTC()
	acc := &domain.Account{
		ID:          "acc-extract",
		Email:       "extract@example.com",
		AccessToken: "token-extract",
		IsActive:    true,
		Status:      domain.AccountStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := env.accountRepo.Create(ctx, acc); err != nil {
		t.Fatalf("create account: %v", err)
	}

	var n atomic.Int32
	env.handler.extractModel = func(r *http.Request, b []byte) (string, ModelCategory, ModelSource) {
		n.Add(1)
		return ExtractModelFromRequest(r, b)
	}

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	reqs := env.mockGoogle.GetRecordedRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 upstream request, got %d", len(reqs))
	}
	return n.Load(), reqs[0].Path
}

// The request is unchanged between the initial scan and the predictive
// fallback check, so the model must be extracted once, not once per site.
func TestProxy_ExtractsModelOncePerRequest(t *testing.T) {
	env := newFallbackTestEnv(t, "gemini-2.5-pro", "gemini-2.5-flash", true)

	scans, _ := serveCountingExtracts(t, env,
		"/v1internal/models/gemini-2.5-pro:streamGenerateContent?alt=sse",
		`{"model":"gemini-2.5-pro","contents":[{"parts":[{"text":"Hello"}]}]}`)

	if scans != 1 {
		t.Fatalf("model extracted %d times, want 1", scans)
	}
}

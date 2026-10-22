package captureapi

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestCaptureParsesBusinessRejectionBeforeStatus(t *testing.T) {
	client := NewClient("https://example.test", "test-key", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Path != capturePath {
			t.Fatalf("request = %s %s", req.Method, req.URL.Path)
		}
		if req.Header.Get("Authorization") != "Bearer test-key" || req.Header.Get("Idempotency-Key") != "run-1:pay-1" {
			t.Fatal("required request headers are missing")
		}
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"ok":false,"data":null,"error":{"code":"REJECTED","message":"invalid capture"},"metadata":{}}`)),
		}, nil
	})})

	_, err := client.CaptureFailure(context.Background(), "run-1:pay-1", CaptureInput{Title: "payment failed"})
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != "REJECTED" || apiErr.Status != http.StatusBadRequest {
		t.Fatalf("error = %#v", err)
	}
}

func TestCaptureRetriesRateLimit(t *testing.T) {
	calls := 0
	client := NewClient("https://example.test", "test-key", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		status, body := http.StatusTooManyRequests, `{"ok":false,"error":{"code":"RATE_LIMITED","message":"retry later"}}`
		if calls == 2 {
			status, body = http.StatusOK, `{"ok":true,"data":{"event_id":"evt-1"},"error":null,"metadata":{}}`
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": []string{"0"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	client.sleep = func(context.Context, time.Duration) error { return nil }

	if _, err := client.CaptureFailure(context.Background(), "run-2:pay-2", CaptureInput{Title: "payment failed"}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

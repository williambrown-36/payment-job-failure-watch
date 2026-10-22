package captureapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const capturePath = "/v1/errors/capture"

type Client struct {
	baseURL string
	key     string
	http    *http.Client
	sleep   func(context.Context, time.Duration) error
}

type CaptureInput struct {
	Title       string         `json:"title"`
	Message     string         `json:"message"`
	Level       string         `json:"level"`
	Fingerprint []string       `json:"fingerprint"`
	Exception   string         `json:"exception"`
	Context     map[string]any `json:"context"`
}

type Envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *APIError       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Infrai %s: %s", e.Code, e.Message)
}

func NewClient(baseURL, key string, httpClient *http.Client) *Client {
	return &Client{baseURL: baseURL, key: key, http: httpClient, sleep: sleepContext}
}

// CaptureFailure uses infrai.errors.capture through its REST path.
func (c *Client) CaptureFailure(ctx context.Context, idempotencyKey string, input CaptureInput) (Envelope, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return Envelope{}, fmt.Errorf("encode capture: %w", err)
	}

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+capturePath, bytes.NewReader(payload))
		if err != nil {
			return Envelope{}, fmt.Errorf("build capture request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)

		res, err := c.http.Do(req)
		if err != nil {
			return Envelope{}, fmt.Errorf("send capture request: %w", err)
		}
		body, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return Envelope{}, fmt.Errorf("read capture response: %w", readErr)
		}

		var env Envelope
		if err := json.Unmarshal(body, &env); err != nil {
			return Envelope{}, fmt.Errorf("decode capture envelope: %w", err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
				return Envelope{}, err
			}
			continue
		}
		if !env.OK {
			if env.Error == nil {
				return Envelope{}, fmt.Errorf("capture rejected with HTTP %d", res.StatusCode)
			}
			env.Error.Status = res.StatusCode
			return Envelope{}, env.Error
		}
		if res.StatusCode >= 500 {
			return Envelope{}, fmt.Errorf("capture transport status %d", res.StatusCode)
		}
		return env, nil
	}
	return Envelope{}, errors.New("capture retry budget exhausted")
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * time.Second
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

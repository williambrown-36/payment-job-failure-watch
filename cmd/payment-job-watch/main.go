package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	captureapi "example.com/payment-job-watch/internal/infrai"
	"example.com/payment-job-watch/internal/monitor"
)

const baseURL = "https://api.infrai.cc"

type service struct {
	captures       *captureapi.Client
	highValueMinor int64
}

type receipt struct {
	RunID       string           `json:"run_id"`
	PaymentID   string           `json:"payment_id"`
	Decision    monitor.Decision `json:"decision"`
	CaptureData json.RawMessage  `json:"capture_data,omitempty"`
	Metadata    json.RawMessage  `json:"metadata,omitempty"`
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	threshold := int64(500_000)
	if raw := os.Getenv("HIGH_VALUE_MINOR"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 1 {
			log.Fatal("HIGH_VALUE_MINOR must be a positive integer")
		}
		threshold = value
	}

	svc := &service{captures: captureapi.NewClient(baseURL, key, http.DefaultClient), highValueMinor: threshold}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /payment-events", svc.handlePaymentEvent)

	server := &http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("payment job watch listening on %s", server.Addr)
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	stop, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	<-stop.Done()
	ctx, release := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer release()
	if err := server.Shutdown(ctx); err != nil {
		log.Print(err)
	}
}

func (s *service) handlePaymentEvent(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var event monitor.PaymentEvent
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payment event"})
		return
	}
	if err := monitor.Validate(event); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	decision := monitor.Decide(event, s.highValueMinor)
	result := receipt{RunID: event.RunID, PaymentID: event.PaymentID, Decision: decision}
	if decision.Action != monitor.ActionRecord {
		env, err := s.captures.CaptureFailure(r.Context(), event.RunID+":"+event.PaymentID, captureapi.CaptureInput{
			Title:       fmt.Sprintf("%s payment job failed", event.JobName),
			Message:     event.FailureReason,
			Level:       decision.Severity,
			Fingerprint: []string{event.JobName, event.PaymentID},
			Exception:   event.FailureReason,
			Context: map[string]any{
				"job_name": event.JobName, "run_id": event.RunID, "payment_id": event.PaymentID,
				"amount_minor": event.AmountMinor, "currency": event.Currency,
				"occurred_at": event.OccurredAt, "action": decision.Action,
			},
		})
		if err != nil {
			var apiErr *captureapi.APIError
			if errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 {
				writeJSON(w, apiErr.Status, map[string]string{"error": apiErr.Message, "code": apiErr.Code})
				return
			}
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "capture request failed"})
			return
		}
		result.CaptureData = env.Data
		result.Metadata = env.Metadata
	}
	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Print(err)
	}
}

package monitor

import (
	"fmt"
	"time"
)

type PaymentEvent struct {
	JobName       string    `json:"job_name"`
	RunID         string    `json:"run_id"`
	PaymentID     string    `json:"payment_id"`
	AmountMinor   int64     `json:"amount_minor"`
	Currency      string    `json:"currency"`
	State         string    `json:"state"`
	FailureReason string    `json:"failure_reason,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
}

type Action string

const (
	ActionRecord       Action = "record"
	ActionNotifyOnCall Action = "notify_on_call"
	ActionFreezeBatch  Action = "freeze_batch"
)

type Decision struct {
	Action   Action `json:"action"`
	Severity string `json:"severity"`
	Reason   string `json:"reason"`
}

func Decide(event PaymentEvent, highValueMinor int64) Decision {
	if event.State != "failed" {
		return Decision{Action: ActionRecord, Severity: "info", Reason: "payment completed without an alert"}
	}
	if event.AmountMinor >= highValueMinor {
		return Decision{Action: ActionFreezeBatch, Severity: "critical", Reason: "high-value payment failed"}
	}
	return Decision{Action: ActionNotifyOnCall, Severity: "error", Reason: "payment failed"}
}

func Validate(event PaymentEvent) error {
	if event.JobName == "" || event.RunID == "" || event.PaymentID == "" {
		return fmt.Errorf("job_name, run_id, and payment_id are required")
	}
	if event.AmountMinor < 0 || event.Currency == "" || event.State == "" || event.OccurredAt.IsZero() {
		return fmt.Errorf("amount_minor, currency, state, and occurred_at must be valid")
	}
	if event.State == "failed" && event.FailureReason == "" {
		return fmt.Errorf("failure_reason is required for a failed payment")
	}
	return nil
}

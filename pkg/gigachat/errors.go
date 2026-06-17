package gigachat

import (
	"errors"
	"fmt"
)

// PaymentRequiredError represents a 402 response from GigaChat.
type PaymentRequiredError struct {
	StatusCode int
	Message    string
	Response   string
}

func (e *PaymentRequiredError) Error() string {
	if e == nil {
		return "gigachat error status 402: Payment Required"
	}
	msg := e.Message
	if msg == "" {
		msg = "Payment Required"
	}
	if e.Response == "" {
		return fmt.Sprintf("gigachat error status %d: %s", e.StatusCode, msg)
	}
	return fmt.Sprintf("gigachat error status %d: %s. Response: %s", e.StatusCode, msg, e.Response)
}

// IsPaymentRequired checks if the error is a PaymentRequiredError.
func IsPaymentRequired(err error) bool {
	var target *PaymentRequiredError
	return errors.As(err, &target)
}

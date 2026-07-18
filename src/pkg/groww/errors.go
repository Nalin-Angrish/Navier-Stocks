package groww

import (
	"errors"
	"fmt"
)

// Sentinel errors returned by the Groww client.
var (
	// ErrAuthExpired is returned when the access token has expired (HTTP 401)
	// and cannot be refreshed.
	ErrAuthExpired = errors.New("groww: access token expired")

	// ErrRateLimited is returned when the API rate limit is exceeded (HTTP 429).
	ErrRateLimited = errors.New("groww: rate limit exceeded")

	// ErrOrderRejected is returned when Groww rejects an order.
	ErrOrderRejected = errors.New("groww: order rejected")

	// ErrInsufficientMargin is returned when there is not enough margin.
	ErrInsufficientMargin = errors.New("groww: insufficient margin")

	// ErrInvalidSymbol is returned when the trading symbol is unknown.
	ErrInvalidSymbol = errors.New("groww: invalid trading symbol")
)

// APIError wraps an unsuccessful Groww API response (status FAILURE) with
// the HTTP status code and a human-readable message from the server.
type APIError struct {
	HTTPStatus int    // HTTP status code (e.g. 400, 401, 429, 500)
	Code       string // Groww error code, if available
	Message    string // Human-readable error message
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("groww: HTTP %d (%s): %s", e.HTTPStatus, e.Code, e.Message)
	}
	return fmt.Sprintf("groww: HTTP %d: %s", e.HTTPStatus, e.Message)
}

// Unwrap enables errors.Is / errors.As on the HTTP status.
func (e *APIError) Unwrap() error {
	switch {
	case e.HTTPStatus >= 500:
		return nil
	case e.HTTPStatus == 429:
		return ErrRateLimited
	case e.HTTPStatus == 401:
		return ErrAuthExpired
	default:
		return nil
	}
}

// IsRateLimited reports whether err indicates a rate-limit response.
func IsRateLimited(err error) bool {
	return errors.Is(err, ErrRateLimited)
}

// IsAuthExpired reports whether err indicates an expired auth token.
func IsAuthExpired(err error) bool {
	return errors.Is(err, ErrAuthExpired)
}

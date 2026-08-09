package riskmanager

import (
	"errors"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/utils"
)

// ErrOpeningBuffer is returned when an intent arrives during the 09:15–09:30
// IST opening buffer, a period of volatile price action the system chooses to
// sit out.
var ErrOpeningBuffer = errors.New("market opening buffer (09:15–09:30 IST)")

// ErrClosingCutoff is returned when an intent arrives at or after the 15:00
// IST closing cutoff; new entries are no longer accepted so positions can be
// squared off before the close.
var ErrClosingCutoff = errors.New("after closing cutoff (15:00 IST)")

// timeWindowGuard rejects intents outside the permitted trading window:
//   - the opening buffer 09:15–09:30 IST (volatile market-open gap), and
//   - anything at or past the 15:00 IST closing cutoff.
//
// It reuses the shared Indian-market-time helpers in pkg/utils.  The explicit
// `now` parameter keeps the gate pure and unit-testable with fixed clocks.
func timeWindowGuard(now time.Time) error {
	switch {
	case utils.IsOpeningBuffer(now):
		return ErrOpeningBuffer
	case utils.IsClosingCutoff(now):
		return ErrClosingCutoff
	default:
		return nil
	}
}

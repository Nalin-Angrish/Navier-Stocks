package riskmanager

import (
	"errors"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

// DefaultMaxBiasRatio is the Gate 3 ceiling on the long/short positional
// ratio, per the Risk-Guardrails HLD: neither market vector may exceed 3:1.
const DefaultMaxBiasRatio = 3.0

// ErrBiasLimit is returned by the directional-bias gate when a prospective
// entry would push the long/short ratio beyond the configured maximum.
var ErrBiasLimit = errors.New("directional bias limit exceeded (3:1)")

// biasGate implements Gate 3 — directional bias control.  It reports whether
// opening a new position of the given side would push the long/short ratio
// beyond the configured maximum in either direction: after the hypothetical
// entry, longs may not exceed maxRatio×shorts, and shorts may not exceed
// maxRatio×longs.  A side with zero open positions imposes no ratio
// constraint (the concurrency ceiling already caps the total footprint).
// CheckBias is the exported form used by tests and callers.
func biasGate(longs, shorts int, side models.Side, maxRatio float64) error {
	return CheckBias(longs, shorts, side, maxRatio)
}

// CheckBias is the pure Gate 3 predicate; see biasGate.
func CheckBias(longs, shorts int, side models.Side, maxRatio float64) error {
	candidateLongs, candidateShorts := longs, shorts
	if side == models.SideShort {
		candidateShorts++
	} else {
		candidateLongs++
	}

	if candidateLongs > 0 && candidateShorts > 0 {
		if float64(candidateLongs) > maxRatio*float64(candidateShorts) {
			return ErrBiasLimit
		}
		if float64(candidateShorts) > maxRatio*float64(candidateLongs) {
			return ErrBiasLimit
		}
	}
	return nil
}

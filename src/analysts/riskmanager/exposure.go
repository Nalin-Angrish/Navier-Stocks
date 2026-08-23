package riskmanager

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

// Gate limits for the risk manager's exposure controls, matching the
// Risk-Guardrails HLD: at most one open trade per sector and four concurrent
// positions platform-wide.
const (
	DefaultMaxSectorPositions = 1
	DefaultMaxTotalPositions  = 4
)

// ErrSectorLimit is returned by Exposure.EntryGate when Gate 1 (sector
// concentration cap) rejects an intent because the sector already holds its
// maximum number of open trades.
var ErrSectorLimit = errors.New("sector concentration limit reached")

// ErrTotalLimit is returned by Exposure.EntryGate when Gate 2 (concurrency
// ceiling) rejects an intent because the platform already holds its maximum
// number of concurrent positions.
var ErrTotalLimit = errors.New("concurrency ceiling reached")

// Exposure is the risk manager's concurrent open-position tracker, per the
// Risk-Guardrails Layer 2 design.  Open-position volumes per sector are kept
// in a thread-safe sync.Map so multiple asynchronous intents arriving over
// NATS can query and mutate state without contention; the global totals are
// tracked with atomic counters.
type Exposure struct {
	sectorCount sync.Map // sector → *atomic.Int64 holding the open count

	total  *atomic.Int64 // concurrent positions platform-wide
	longs  *atomic.Int64 // concurrent long positions
	shorts *atomic.Int64 // concurrent short positions

	maxSector int     // Gate 1: max open trades per sector
	maxTotal  int     // Gate 2: max concurrent positions platform-wide
	maxBias   float64 // Gate 3: max long/short positional ratio
}

// NewExposure creates an Exposure with the given Gate 1 / Gate 2 ceilings.
// Gate 3 defaults to the Risk-Guardrails 3:1 directional ceiling.
func NewExposure(maxSector, maxTotal int) *Exposure {
	return &Exposure{
		maxSector: maxSector,
		maxTotal:  maxTotal,
		maxBias:   DefaultMaxBiasRatio,
		total:     &atomic.Int64{},
		longs:     &atomic.Int64{},
		shorts:    &atomic.Int64{},
	}
}

// EntryGate runs Gate 1 and Gate 2 for a prospective entry in the given
// sector.  It returns an error identifying the first violated limit, or nil
// when the entry is permitted.
func (e *Exposure) EntryGate(sector string) error {
	if cur, ok := e.sectorCount.Load(sector); ok && cur.(*atomic.Int64).Load() >= int64(e.maxSector) {
		return ErrSectorLimit
	}
	if e.total.Load() >= int64(e.maxTotal) {
		return ErrTotalLimit
	}
	return nil
}

// Register records a new open position in the given sector.  The side is
// tallied so the directional-bias layer can enforce its long/short ratio.
func (e *Exposure) Register(side models.Side, sector string) {
	e.total.Add(1)
	if side == models.SideShort {
		e.shorts.Add(1)
	} else {
		e.longs.Add(1)
	}
	cur, _ := e.sectorCount.LoadOrStore(sector, &atomic.Int64{})
	cur.(*atomic.Int64).Add(1)
}

// Release frees one open position in the given sector, called when a position
// is squared off or closed.  It never underflows: closing below zero is
// ignored.
func (e *Exposure) Release(side models.Side, sector string) {
	if cur, ok := e.sectorCount.Load(sector); ok {
		if cur.(*atomic.Int64).Add(-1) <= 0 {
			e.sectorCount.Delete(sector)
		}
	}
	if e.total.Add(-1) < 0 {
		e.total.Add(1) // undo accidental underflow
	}
	if side == models.SideShort {
		if e.shorts.Add(-1) < 0 {
			e.shorts.Add(1)
		}
	} else {
		if e.longs.Add(-1) < 0 {
			e.longs.Add(1)
		}
	}
}

// SectorCount returns how many open positions are held in the given sector.
func (e *Exposure) SectorCount(sector string) int {
	if cur, ok := e.sectorCount.Load(sector); ok {
		return int(cur.(*atomic.Int64).Load())
	}
	return 0
}

// Total returns the number of concurrent open positions platform-wide.
func (e *Exposure) Total() int {
	return int(e.total.Load())
}

// Longs returns the number of concurrent open long positions.
func (e *Exposure) Longs() int {
	return int(e.longs.Load())
}

// Shorts returns the number of concurrent open short positions.
func (e *Exposure) Shorts() int {
	return int(e.shorts.Load())
}

// MaxBiasRatio returns the Gate 3 long/short ceiling in use by this exposure.
func (e *Exposure) MaxBiasRatio() float64 {
	return e.maxBias
}

// RefreshFromDB recalculates all exposure counters from the actual open
// positions in the database, syncing the in-memory state with the source
// of truth.  This is called periodically to account for intraday exits
// (stop-loss, take-profit) that bypass the RM's Release path.
func (e *Exposure) RefreshFromDB(positions []models.Position) {
	// Reset all counters.
	e.total.Store(0)
	e.longs.Store(0)
	e.shorts.Store(0)
	e.sectorCount.Range(func(key, value interface{}) bool {
		e.sectorCount.Delete(key)
		return true
	})

	// Recount from actual open positions.
	for _, pos := range positions {
		e.total.Add(1)
		if pos.Side == models.SideShort {
			e.shorts.Add(1)
		} else {
			e.longs.Add(1)
		}
		cur, _ := e.sectorCount.LoadOrStore(pos.Sector, &atomic.Int64{})
		cur.(*atomic.Int64).Add(1)
	}
}

// BiasGate runs Gate 3 — directional bias control — for a prospective entry
// of the given side.  It returns ErrBiasLimit when the long/short ratio would
// exceed the configured ceiling.
func (e *Exposure) BiasGate(side models.Side) error {
	return biasGate(e.Longs(), e.Shorts(), side, e.maxBias)
}

// SectorResolver maps a ticker symbol to its structural sector so the
// exposure gate can evaluate concentration.  Production wiring uses the
// shared universe SectorMap exported by the Quantitative Scout.
type SectorResolver interface {
	Sector(symbol string) string
}

// sectorOf resolves the ticker's sector, defaulting to "UNKNOWN" when the
// resolver is nil or does not know the symbol.
func (g *Analyst) sectorOf(ticker string) string {
	if g.sectors == nil {
		return "UNKNOWN"
	}
	return g.sectors.Sector(ticker)
}

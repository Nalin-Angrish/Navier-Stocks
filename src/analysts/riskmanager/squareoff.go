package riskmanager

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

// DefaultSquareOffTime is the IST wall-clock trigger for the auto-square-off
// routine: all outstanding MIS positions must be liquidated by 3:15 PM so the
// broker does not auto-close them with penalty fees.
var DefaultSquareOffTime = time.Date(0, 1, 1, 15, 15, 0, 0, time.UTC)

// SquareOffInterval is how often the routine checks whether it is time to
// square off.
const SquareOffInterval = 30 * time.Second

// PositionStore is the persistence surface the auto-square-off routine needs:
// enumerate open positions and transition them to CLOSED after a liquidation
// signal is published.
type PositionStore interface {
	ListOpen() ([]models.Position, error)
	MarkClosed(id int64, exitPrice float64, reason models.ExitReason) error
}

// squareOff ensures all open positions are liquidated at the configured
// 15:15 IST trigger.  It is safe to call repeatedly: it only acts once per
// trading day, and only after the trigger time has passed.
func (g *Analyst) squareOff(now time.Time) {
	if g.positions == nil {
		return
	}

	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		log.Printf("[Risk Manager] square-off: timezone error: %v", err)
		return
	}
	now = now.In(loc)

	trigger := DefaultSquareOffTime
	trigger = time.Date(now.Year(), now.Month(), now.Day(),
		trigger.Hour(), trigger.Minute(), 0, 0, loc)

	day := now.Format("2006-01-02")
	if now.Before(trigger) {
		return // not yet; quiet until trigger time
	}
	if g.sqDone == day {
		return // already squared off today
	}

	if err := g.liquidate(now); err != nil {
		log.Printf("[Risk Manager] square-off failed: %v", err)
		return
	}
	// Only mark the day as done when ALL positions were successfully
	// closed.  Partial failures will retry on the next tick.
	if g.allClosed {
		g.sqDone = day
		log.Printf("[Risk Manager] auto square-off complete (%s)", day)
	}
}

// liquidate enumerates open positions, publishes a market-square-off signal
// for each, and marks them CLOSED in the database.
func (g *Analyst) liquidate(now time.Time) error {
	open, err := g.positions.ListOpen()
	if err != nil {
		return fmt.Errorf("list open positions: %w", err)
	}
	if len(open) == 0 {
		g.allClosed = true
		return nil
	}

	allClosed := true
	for i := range open {
		pos := &open[i]
		exitPrice := g.LatestPrice(pos.Ticker)
		if g.PriceStale(pos.Ticker, 2*SquareOffInterval) {
			log.Printf("[Risk Manager] WARNING: price for %s is stale — falling back to entry price", pos.Ticker)
		}
		if exitPrice <= 0 {
			exitPrice = pos.EntryPrice // fallback when no tick received
		}
		if err := g.signalSquareOff(pos, exitPrice, now); err != nil {
			log.Printf("[Risk Manager] square-off %s %s: %v", pos.Side, pos.Ticker, err)
			allClosed = false
			continue // keep the position open so it can retry
		}
		if err := g.positions.MarkClosed(pos.ID, exitPrice, models.ReasonSquareOff); err != nil {
			log.Printf("[Risk Manager] mark %s closed: %v", pos.Ticker, err)
			allClosed = false
			continue
		}
		if g.exposure != nil {
			g.exposure.Release(pos.Side, pos.Sector)
		}
		log.Printf("[Risk Manager] squared off %s %s", pos.Side, pos.Ticker)
	}
	g.allClosed = allClosed
	return nil
}

// signalSquareOff publishes a market-square-off execution for the position: a
// closing order on the opposite side at the marked-down/up price.  The
// ExecutionRef ties the liquidation back to the original position.
func (g *Analyst) signalSquareOff(pos *models.Position, exitPrice float64, now time.Time) error {
	if g.js == nil {
		return nil // no broker link (unit tests): nothing to publish
	}

	side := models.SideLong
	if pos.Side == models.SideLong {
		side = models.SideShort
	}

	exec := &models.TradeExecution{
		Ticker:       pos.Ticker,
		Side:         side,
		Quantity:     pos.Quantity,
		Price:        exitPrice,
		Sector:       pos.Sector,
		SignalReason: "auto_square_off",
		ExecutionRef: fmt.Sprintf("sqoff-%s-%d", pos.ExecutionRef, now.UnixNano()),
		PositionID:   fmt.Sprintf("%d", pos.ID),
	}
	data, err := json.Marshal(exec)
	if err != nil {
		return fmt.Errorf("marshal square-off: %w", err)
	}
	if err := g.js.Publish(ExecuteSubject(pos.Ticker), data); err != nil {
		return fmt.Errorf("publish %s: %w", pos.Ticker, err)
	}
	return nil
}

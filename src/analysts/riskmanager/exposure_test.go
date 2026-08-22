package riskmanager_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/riskmanager"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

// staticSectors is a fixed ticker→sector resolver for gate tests.
type staticSectors map[string]string

func (s staticSectors) Sector(symbol string) string {
	if sec, ok := s[symbol]; ok {
		return sec
	}
	return "UNKNOWN"
}

func TestExposure_EntryGate_PassesEmptyExposure(t *testing.T) {
	exp := riskmanager.NewExposure(1, 4)
	if err := exp.EntryGate("IT"); err != nil {
		t.Fatalf("expected no gate error on empty exposure, got %v", err)
	}
}

func TestExposure_Gate1_SectorCap(t *testing.T) {
	exp := riskmanager.NewExposure(1, 4)
	exp.Register(models.SideLong, "IT")

	if err := exp.EntryGate("IT"); !errors.Is(err, riskmanager.ErrSectorLimit) {
		t.Fatalf("expected ErrSectorLimit for occupied sector, got %v", err)
	}
	// A different sector is still fine under Gate 1.
	if err := exp.EntryGate("BANKING"); err != nil {
		t.Fatalf("unexpected error for free sector: %v", err)
	}
}

func TestExposure_Gate2_ConcurrencyCeiling(t *testing.T) {
	exp := riskmanager.NewExposure(1, 2)
	exp.Register(models.SideLong, "IT")
	exp.Register(models.SideLong, "AUTO")

	if err := exp.EntryGate("METAL"); !errors.Is(err, riskmanager.ErrTotalLimit) {
		t.Fatalf("expected ErrTotalLimit at ceiling, got %v", err)
	}
}

func TestExposure_RegisterRelease(t *testing.T) {
	exp := riskmanager.NewExposure(1, 4)
	exp.Register(models.SideLong, "IT")
	exp.Register(models.SideShort, "BANKING")

	if exp.Total() != 2 {
		t.Fatalf("Total() = %d, want 2", exp.Total())
	}
	if exp.SectorCount("IT") != 1 {
		t.Fatalf("SectorCount(IT) = %d, want 1", exp.SectorCount("IT"))
	}

	exp.Release(models.SideLong, "IT")
	if exp.Total() != 1 {
		t.Fatalf("Total() after release = %d, want 1", exp.Total())
	}
	if exp.SectorCount("IT") != 0 {
		t.Fatalf("SectorCount(IT) after release = %d, want 0", exp.SectorCount("IT"))
	}

	// Freeing a sector reopens it to new entries.
	if err := exp.EntryGate("IT"); err != nil {
		t.Fatalf("expected IT to be reopenable after release, got %v", err)
	}
}

func TestExposure_ReleaseDoesNotUnderflow(t *testing.T) {
	exp := riskmanager.NewExposure(1, 4)
	exp.Release(models.SideLong, "IT")
	if exp.Total() != 0 {
		t.Fatalf("Total() = %d, want 0 (no underflow)", exp.Total())
	}
}

func TestExposure_ConcurrentRegister(t *testing.T) {
	exp := riskmanager.NewExposure(4, 100)

	const workers = 16
	const perWorker = 50
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				exp.Register(models.SideLong, "IT")
			}
		}()
	}
	wg.Wait()

	if got := exp.SectorCount("IT"); got != workers*perWorker {
		t.Fatalf("SectorCount(IT) after concurrency = %d, want %d", got, workers*perWorker)
	}
	if got := exp.Total(); got != workers*perWorker {
		t.Fatalf("Total() after concurrency = %d, want %d", got, workers*perWorker)
	}
}

func TestAnalyse_ExposureGateWired(t *testing.T) {
	agent := riskmanager.NewAgentForTest(nil)
	exp := riskmanager.NewExposure(1, 4)
	agent.SetSectors(staticSectors{"RELIANCE": "ENERGY"})
	agent.SetExposure(exp)

	// First entry in the sector is allowed past the exposure gate.
	if err := agent.EvaluateRaw(models.TradeIntent{Ticker: "RELIANCE", Side: models.SideLong}); err != nil {
		t.Fatalf("expected first entry to pass, got %v", err)
	}
	exp.Register(models.SideLong, "ENERGY")

	err := agent.EvaluateRaw(models.TradeIntent{Ticker: "RELIANCE", Side: models.SideLong})
	if !errors.Is(err, riskmanager.ErrSectorLimit) {
		t.Fatalf("expected ErrSectorLimit for repeated sector, got %v", err)
	}
}

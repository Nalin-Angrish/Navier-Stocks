package quantitative_test

import (
	"os"
	"testing"

	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/quantitative"
)

func TestDefaultUniverse_HasExpectedSymbols(t *testing.T) {
	u := quantitative.DefaultUniverse()

	expected := []string{
		"TCS", "INFY", "WIPRO", "HCLTECH", "TECHM",
		"HDFCBANK", "ICICIBANK", "SBIN", "KOTAKBANK", "AXISBANK",
		"TATAMOTORS", "M&M", "BAJAJ-AUTO", "MARUTI", "EICHERMOT",
		"RELIANCE", "ONGC", "NTPC", "POWERGRID", "ADANIENT",
		"ITC", "HUL", "BRITANNIA", "NESTLEIND", "TITAN",
	}

	if len(u.Symbols) != len(expected) {
		t.Fatalf("got %d symbols, want %d", len(u.Symbols), len(expected))
	}

	seen := make(map[string]bool)
	for _, s := range u.Symbols {
		seen[s] = true
	}
	for _, s := range expected {
		if !seen[s] {
			t.Fatalf("missing default symbol %q", s)
		}
	}
}

func TestDefaultUniverse_SectorMapping(t *testing.T) {
	u := quantitative.DefaultUniverse()

	tests := []struct {
		symbol, sector string
	}{
		{"TCS", "IT"},
		{"INFY", "IT"},
		{"WIPRO", "IT"},
		{"HCLTECH", "IT"},
		{"TECHM", "IT"},
		{"HDFCBANK", "BANKING"},
		{"ICICIBANK", "BANKING"},
		{"SBIN", "BANKING"},
		{"KOTAKBANK", "BANKING"},
		{"AXISBANK", "BANKING"},
		{"TATAMOTORS", "AUTO"},
		{"M&M", "AUTO"},
		{"BAJAJ-AUTO", "AUTO"},
		{"MARUTI", "AUTO"},
		{"EICHERMOT", "AUTO"},
		{"RELIANCE", "ENERGY"},
		{"ONGC", "ENERGY"},
		{"NTPC", "ENERGY"},
		{"POWERGRID", "ENERGY"},
		{"ADANIENT", "ENERGY"},
		{"ITC", "FMCG"},
		{"HUL", "FMCG"},
		{"BRITANNIA", "FMCG"},
		{"NESTLEIND", "FMCG"},
		{"TITAN", "FMCG"},
	}

	for _, tt := range tests {
		if got := u.SymbolToSector[tt.symbol]; got != tt.sector {
			t.Errorf("SymbolToSector[%q] = %q, want %q", tt.symbol, got, tt.sector)
		}
	}
}

func TestDefaultUniverse_EntryCount(t *testing.T) {
	u := quantitative.DefaultUniverse()
	if len(u.Entries) != len(u.Symbols) {
		t.Fatalf("Entries (%d) and Symbols (%d) length mismatch", len(u.Entries), len(u.Symbols))
	}
}

func TestResolveUniverse_EnvOverride(t *testing.T) {
	t.Setenv("QUANT_UNIVERSE", "TATASTEEL, RELIANCE, ITC")

	u := quantitative.ResolveUniverse()
	if len(u.Symbols) != 3 {
		t.Fatalf("got %d symbols, want 3", len(u.Symbols))
	}
	if u.Symbols[0] != "TATASTEEL" || u.Symbols[1] != "RELIANCE" || u.Symbols[2] != "ITC" {
		t.Fatalf("symbols = %v, want [TATASTEEL RELIANCE ITC]", u.Symbols)
	}
}

func TestResolveUniverse_EnvPreservesSector(t *testing.T) {
	t.Setenv("QUANT_UNIVERSE", "TCS, UNKNOWNSYM")

	u := quantitative.ResolveUniverse()
	if u.SymbolToSector["TCS"] != "IT" {
		t.Fatalf("TCS sector = %q, want IT", u.SymbolToSector["TCS"])
	}
	if u.SymbolToSector["UNKNOWNSYM"] != "UNKNOWN" {
		t.Fatalf("UNKNOWNSYM sector = %q, want UNKNOWN", u.SymbolToSector["UNKNOWNSYM"])
	}
}

func TestResolveUniverse_EmptyEnv(t *testing.T) {
	t.Setenv("QUANT_UNIVERSE", "")

	u := quantitative.ResolveUniverse()
	if len(u.Symbols) != 25 {
		t.Fatalf("empty env: got %d symbols, want 25 (fallback to default)", len(u.Symbols))
	}
}

func TestResolveUniverse_NoEnv(t *testing.T) {
	t.Setenv("QUANT_UNIVERSE", "TEMP_UNSET")
	if err := os.Unsetenv("QUANT_UNIVERSE"); err != nil {
		t.Fatalf("os.Unsetenv: %v", err)
	}

	u := quantitative.ResolveUniverse()
	if len(u.Symbols) != 25 {
		t.Fatalf("no env: got %d symbols, want 25 (fallback to default)", len(u.Symbols))
	}
}

func TestSectorMap_Sector(t *testing.T) {
	u := quantitative.DefaultUniverse()
	sm := u.AsSectorMap()

	if sm.Sector("TCS") != "IT" {
		t.Fatalf("Sector(TCS) = %q, want IT", sm.Sector("TCS"))
	}
	if sm.Sector("NONEXISTENT") != "UNKNOWN" {
		t.Fatalf("Sector(NONEXISTENT) = %q, want UNKNOWN", sm.Sector("NONEXISTENT"))
	}
}

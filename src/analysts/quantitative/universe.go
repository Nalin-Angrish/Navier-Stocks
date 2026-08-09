package quantitative

import (
	"os"
	"strings"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/groww"
)

// UniverseEntry identifies a single security in the trading universe with its
// exchange, segment, exchange token (for WebSocket subscription), and sector
// tag (for the Risk Manager's sector-concentration gate).
type UniverseEntry struct {
	Symbol        string
	Exchange      groww.Exchange
	Segment       groww.Segment
	ExchangeToken string
	Sector        string
}

// Universe is the resolved pre-market list of securities the Quantitative
// Scout will track.  It provides fast lookups from exchange token → symbol
// and from symbol → sector.
type Universe struct {
	Entries        []UniverseEntry
	Symbols        []string
	TokenToSymbol  map[string]string // exchange_token → trading symbol
	SymbolToSector map[string]string
}

// defaultEntries is the baseline universe defined in the Risk-Guardrails doc:
// 15–25 liquid securities drawn from Nifty sectoral indices.
var defaultEntries = []UniverseEntry{
	// Nifty IT
	{Symbol: "TCS", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "IT"},
	{Symbol: "INFY", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "IT"},
	// Nifty Bank
	{Symbol: "HDFCBANK", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "BANKING"},
	{Symbol: "SBIN", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "BANKING"},
	{Symbol: "ICICIBANK", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "BANKING"},
	// Nifty Auto
	{Symbol: "TATAMOTORS", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "AUTO"},
	{Symbol: "M&M", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "AUTO"},
	// Nifty Metal
	{Symbol: "TATASTEEL", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "METAL"},
	{Symbol: "JSWSTEEL", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "METAL"},
	// Nifty Energy / FMCG
	{Symbol: "RELIANCE", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "ENERGY"},
	{Symbol: "ITC", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "FMCG"},
}

// DefaultUniverse returns the baseline universe of 11 securities from the
// five major Nifty sectoral indices.  Exchange tokens are left empty and
// can be resolved later from the instrument master CSV.
func DefaultUniverse() *Universe {
	return buildUniverse(defaultEntries)
}

// ResolveUniverse loads the trading universe from the QUANT_UNIVERSE
// environment variable (a comma-separated list of symbols) if set, otherwise
// falls back to DefaultUniverse.  Sector tags for custom symbols default to
// "UNKNOWN".
func ResolveUniverse() *Universe {
	raw := os.Getenv("QUANT_UNIVERSE")
	if raw == "" {
		return DefaultUniverse()
	}

	symbols := strings.Split(raw, ",")
	entries := make([]UniverseEntry, 0, len(symbols))
	for _, s := range symbols {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		entries = append(entries, UniverseEntry{
			Symbol:   s,
			Exchange: groww.ExchangeNSE,
			Segment:  groww.SegmentCash,
			Sector:   sectorForSymbol(s),
		})
	}
	if len(entries) == 0 {
		return DefaultUniverse()
	}
	return buildUniverse(entries)
}

// buildUniverse constructs a Universe from a slice of entries, populating
// the lookup maps.
func buildUniverse(entries []UniverseEntry) *Universe {
	u := &Universe{
		Entries:        entries,
		Symbols:        make([]string, len(entries)),
		TokenToSymbol:  make(map[string]string, len(entries)),
		SymbolToSector: make(map[string]string, len(entries)),
	}
	for i, e := range entries {
		u.Symbols[i] = e.Symbol
		u.SymbolToSector[e.Symbol] = e.Sector
		if e.ExchangeToken != "" {
			u.TokenToSymbol[e.ExchangeToken] = e.Symbol
		}
	}
	return u
}

// sectorForSymbol returns the default sector for a known baseline symbol,
// otherwise "UNKNOWN".  This allows custom universe overrides to still
// carry reasonable sector tags.
func sectorForSymbol(symbol string) string {
	for _, e := range defaultEntries {
		if e.Symbol == symbol {
			return e.Sector
		}
	}
	return "UNKNOWN"
}

// SectorMap is a read-only lookup table from symbol → sector that the Risk
// Manager uses during its sector-concentration gate.  The Quantitative Scout
// exports this so the two agents share the same mapping without duplicating
// configuration.
type SectorMap map[string]string

// Sector returns the sector for a symbol, or "UNKNOWN" if the symbol is not
// tracked.
func (sm SectorMap) Sector(symbol string) string {
	if s, ok := sm[symbol]; ok {
		return s
	}
	return "UNKNOWN"
}

// AsSectorMap converts the universe's symbol→sector mapping into a
// SectorMap for export to the Risk Manager.
func (u *Universe) AsSectorMap() SectorMap {
	return SectorMap(u.SymbolToSector)
}

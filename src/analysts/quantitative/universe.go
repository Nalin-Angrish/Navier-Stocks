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

// defaultEntries is the baseline universe: 25 liquid Nifty constituents
// across5 sectors chosen for intraday momentum breakout trading.
var defaultEntries = []UniverseEntry{
	// Nifty IT (5)
	{Symbol: "TCS", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "IT"},
	{Symbol: "INFY", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "IT"},
	{Symbol: "WIPRO", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "IT"},
	{Symbol: "HCLTECH", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "IT"},
	{Symbol: "TECHM", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "IT"},
	// Nifty Bank (5)
	{Symbol: "HDFCBANK", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "BANKING"},
	{Symbol: "ICICIBANK", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "BANKING"},
	{Symbol: "SBIN", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "BANKING"},
	{Symbol: "KOTAKBANK", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "BANKING"},
	{Symbol: "AXISBANK", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "BANKING"},
	// Nifty Auto (5)
	{Symbol: "TATAMOTORS", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "AUTO"},
	{Symbol: "M&M", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "AUTO"},
	{Symbol: "BAJAJ-AUTO", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "AUTO"},
	{Symbol: "MARUTI", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "AUTO"},
	{Symbol: "EICHERMOT", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "AUTO"},
	// Nifty Energy (5)
	{Symbol: "RELIANCE", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "ENERGY"},
	{Symbol: "ONGC", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "ENERGY"},
	{Symbol: "NTPC", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "ENERGY"},
	{Symbol: "POWERGRID", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "ENERGY"},
	{Symbol: "ADANIENT", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "ENERGY"},
	// Nifty FMCG (5)
	{Symbol: "ITC", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "FMCG"},
	{Symbol: "HUL", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "FMCG"},
	{Symbol: "BRITANNIA", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "FMCG"},
	{Symbol: "NESTLEIND", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "FMCG"},
	{Symbol: "TITAN", Exchange: groww.ExchangeNSE, Segment: groww.SegmentCash, Sector: "FMCG"},
}

// DefaultUniverse returns the baseline universe of 25 securities from
// 5 Nifty sectoral indices.  Exchange tokens are left empty and
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

// NewUniverseFromSymbols creates a Universe for the given symbols (NSE/Cash)
// with sector tags inferred from defaultEntries or UNKNOWN. Exported for
// tools and tests.
func NewUniverseFromSymbols(symbols []string) *Universe {
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

// ResolveTokens fills ExchangeToken for each entry via the Groww
// instrument CSV (public, no auth). It is idempotent — already-filled
// entries are left intact. This is required for the NATS feed where the
// subject is /ld/eq/nse/price.<token>; without tokens subscriptions are
// invalid (see feedcheck's resolveTokens for the same logic).
func (u *Universe) ResolveTokens() error {
	if len(u.Entries) == 0 {
		return nil
	}
	// Fast path: all tokens already present
	allFilled := true
	for _, e := range u.Entries {
		if e.ExchangeToken == "" {
			allFilled = false
			break
		}
	}
	if allFilled {
		return nil
	}
	raw, err := groww.DownloadInstruments()
	if err != nil {
		return err
	}
	instruments, err := groww.ParseInstruments(raw)
	if err != nil {
		return err
	}
	tokenBySymbol := make(map[string]string, len(instruments))
	for _, inst := range instruments {
		if inst.Exchange != groww.ExchangeNSE || inst.Segment != groww.SegmentCash {
			continue
		}
		if _, ok := tokenBySymbol[inst.TradingSymbol]; !ok && inst.ExchangeToken != "" {
			tokenBySymbol[inst.TradingSymbol] = inst.ExchangeToken
		}
	}
	for i, e := range u.Entries {
		if e.ExchangeToken != "" {
			continue
		}
		if tok, ok := tokenBySymbol[e.Symbol]; ok {
			u.Entries[i].ExchangeToken = tok
			u.TokenToSymbol[tok] = e.Symbol
		}
	}
	return nil
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

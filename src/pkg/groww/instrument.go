package groww

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// DefaultInstrumentURL is the URL of the Groww instruments CSV file.
const DefaultInstrumentURL = "https://growwapi-assets.groww.in/instruments/instrument.csv"

// DownloadInstruments fetches the instrument CSV from the Groww assets CDN.
func DownloadInstruments() ([]byte, error) {
	resp, err := http.Get(DefaultInstrumentURL)
	if err != nil {
		return nil, fmt.Errorf("groww instruments download: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("groww instruments: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("groww instruments read: %w", err)
	}
	return data, nil
}

// ParseInstruments parses the instrument CSV into a slice of Instrument.
// The CSV is expected to have a header row matching the Instrument fields.
func ParseInstruments(data []byte) ([]Instrument, error) {
	r := csv.NewReader(strings.NewReader(string(data)))

	// Read header row.
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("groww instruments header: %w", err)
	}

	// Build a column index.
	col := make(map[string]int, len(header))
	for i, name := range header {
		col[name] = i
	}

	required := []string{"exchange", "exchange_token", "trading_symbol", "instrument_type", "segment"}
	for _, name := range required {
		if _, ok := col[name]; !ok {
			return nil, fmt.Errorf("groww instruments: missing column %q", name)
		}
	}

	var instruments []Instrument
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("groww instruments row: %w", err)
		}

		inst := Instrument{
			Exchange:                Exchange(rowAt(row, col, "exchange")),
			ExchangeToken:           rowAt(row, col, "exchange_token"),
			TradingSymbol:           rowAt(row, col, "trading_symbol"),
			GrowwSymbol:             rowAt(row, col, "groww_symbol"),
			Name:                    rowAt(row, col, "name"),
			InstrumentType:          InstrumentType(rowAt(row, col, "instrument_type")),
			Segment:                 Segment(rowAt(row, col, "segment")),
			Series:                  rowAt(row, col, "series"),
			ISIN:                    rowAt(row, col, "isin"),
			UnderlyingSymbol:        rowAt(row, col, "underlying_symbol"),
			UnderlyingExchangeToken: rowAt(row, col, "underlying_exchange_token"),
			ExpiryDate:              rowAt(row, col, "expiry_date"),
			StrikePrice:             atoi(rowAt(row, col, "strike_price")),
			LotSize:                 atoi(rowAt(row, col, "lot_size")),
			TickSize:                atof(rowAt(row, col, "tick_size")),
			FreezeQuantity:          atoi(rowAt(row, col, "freeze_quantity")),
			IsReserved:              rowAt(row, col, "is_reserved") == "1",
			BuyAllowed:              rowAt(row, col, "buy_allowed") == "1",
			SellAllowed:             rowAt(row, col, "sell_allowed") == "1",
		}
		instruments = append(instruments, inst)
	}
	return instruments, nil
}

// rowAt safely returns the column value or empty string if the index is out
// of range.
func rowAt(row []string, col map[string]int, name string) string {
	idx, ok := col[name]
	if !ok || idx >= len(row) {
		return ""
	}
	return row[idx]
}

func atoi(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

func atof(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

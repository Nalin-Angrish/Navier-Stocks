package quantitative_test

import (
	"testing"

	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/quantitative"
)

func TestFeedConnector_Creation(t *testing.T) {
	u := quantitative.DefaultUniverse()
	ts := quantitative.NewTickerStore(5, 256)
	fc := quantitative.NewFeedConnector(nil, ts, u)
	if fc == nil {
		t.Fatal("NewFeedConnector returned nil")
	}
	if len(u.Entries) == 0 {
		t.Fatal("universe has no entries")
	}
}

func TestFeedConnector_UniverseEntryCount(t *testing.T) {
	u := quantitative.DefaultUniverse()
	if len(u.Entries) != len(u.Symbols) {
		t.Fatalf("Entries (%d) != Symbols (%d)", len(u.Entries), len(u.Symbols))
	}
}

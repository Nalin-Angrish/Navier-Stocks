package recorder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

func TestEnvTicksDir(t *testing.T) {
	t.Setenv(ticksDirEnvKey, "/tmp/opencode/ticks-custom")
	if got := envTicksDir(); got != "/tmp/opencode/ticks-custom" {
		t.Fatalf("envTicksDir = %q", got)
	}
	t.Setenv(ticksDirEnvKey, "")
	if got := envTicksDir(); got != DefaultTicksDir {
		t.Fatalf("fallback = %q, want %q", got, DefaultTicksDir)
	}
}

// handlePrice is exercised through the real JSON path used by Run():
// valid lines land in the archive, malformed ones are dropped with a log.
func TestHandlePrice_AppendsAndSkipsMalformed(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenTickStore(dir)
	if err != nil {
		t.Fatalf("OpenTickStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	a := newAnalyst(nil, store)

	good, err := json.Marshal(tick("RELIANCE", 2500))
	if err != nil {
		t.Fatalf("marshal tick: %v", err)
	}
	a.handlePrice(&nats.Msg{Data: good})
	a.handlePrice(&nats.Msg{Data: []byte("{not json")})
	a.handlePrice(&nats.Msg{Data: good})

	raw, err := os.ReadFile(filepath.Join(dir, currentFile(store)))
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	var got []models.PriceTick
	start := 0
	for i, b := range raw {
		if b == '\n' {
			var tick models.PriceTick
			if err := json.Unmarshal(raw[start:i], &tick); err != nil {
				t.Fatalf("stored line is not valid JSON: %v", err)
			}
			got = append(got, tick)
			start = i + 1
		}
	}
	if len(got) != 2 || got[0].Ticker != "RELIANCE" || got[1].Price != 2500 {
		t.Fatalf("malformed tick stored or valid ones lost: %+v", got)
	}
}

// currentFile returns the base name of the file the store writes to.
func currentFile(s *TickStore) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return filepath.Base(s.file.Name())
}

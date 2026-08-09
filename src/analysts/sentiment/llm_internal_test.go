package sentiment

import (
	"strings"
	"testing"
)

// TestSystemPromptEmbedded guards against the embedded SYSTEM.md prompt being
// accidentally emptied or losing its contract: the model must be told to emit
// a single parseable JSON object with a bias and a confidence.
func TestSystemPromptEmbedded(t *testing.T) {
	if systemPrompt == "" {
		t.Fatal("embedded system prompt is empty")
	}
	if strings.Contains(systemPrompt, "go:embed") {
		t.Fatal("embedded system prompt looks like it leaked the source directive")
	}
	for _, want := range []string{
		"BULLISH",
		"BEARISH",
		"NEUTRAL",
		"single JSON object",
		"confidence",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("embedded system prompt missing %q", want)
		}
	}
}

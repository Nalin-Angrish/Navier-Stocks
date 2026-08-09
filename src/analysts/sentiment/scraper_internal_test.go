package sentiment

import (
	"testing"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

func TestArticleHash_Deterministic(t *testing.T) {
	a := &models.Article{URL: "https://example.com/x"}
	b := &models.Article{URL: "https://example.com/x"}
	c := &models.Article{URL: "https://example.com/y"}

	if articleHash(a) != articleHash(b) {
		t.Fatal("identical URLs must hash identically")
	}
	if articleHash(a) == articleHash(c) {
		t.Fatal("different URLs must hash differently")
	}
}

func TestArticleHash_FallsBackToTitle(t *testing.T) {
	a := &models.Article{Title: "Some headline", URL: ""}
	b := &models.Article{Title: "Some headline", URL: ""}
	c := &models.Article{Title: "Other headline", URL: ""}

	if articleHash(a) != articleHash(b) {
		t.Fatal("identical titles must hash identically")
	}
	if articleHash(a) == articleHash(c) {
		t.Fatal("different titles must hash differently")
	}
}

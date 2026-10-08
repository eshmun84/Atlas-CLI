package delivery_test

import (
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/delivery"
)

func TestDefaultMode(t *testing.T) {
	t.Parallel()
	if got := delivery.DefaultMode(true); got != delivery.ModeGitLocal {
		t.Fatalf("got %q", got)
	}
	if got := delivery.DefaultMode(false); got != delivery.ModeNone {
		t.Fatalf("got %q", got)
	}
}

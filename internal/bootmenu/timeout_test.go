package bootmenu

import (
	"testing"

	"pxe/internal/storage"
)

func TestTimeoutSeconds(t *testing.T) {
	menu := storage.Menu{TimeoutSeconds: 6}
	if got := TimeoutSeconds(menu); got != 6 {
		t.Fatalf("expected fixed timeout 6, got %d", got)
	}

	menu.RandomizeTimeout = true
	for i := 0; i < 32; i++ {
		got := TimeoutSeconds(menu)
		if got < 0 || got > 6 {
			t.Fatalf("random timeout out of range: %d", got)
		}
	}
}

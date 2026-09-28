package observability

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogRotationBounded(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pxe.log")
	l, err := OpenLog(p)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	chunk := make([]byte, logMaxBytes)
	for i := 0; i < 3; i++ {
		if _, err = l.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{p, p + ".1"} {
		info, err := os.Stat(name)
		if err != nil || info.Size() != logMaxBytes {
			t.Fatalf("rotation %s %v", name, err)
		}
	}
}

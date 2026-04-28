package pxeopt

import (
	"testing"

	"pxe/internal/storage"
)

func TestBuildOption43(t *testing.T) {
	menu := storage.Menu{
		Enabled: true,
		Prompt:  "Boot Menu",
		Items: []storage.MenuItem{
			{Title: "iPXE", PXEType: "8000", ServerIP: "%tftpserver%", Enabled: true},
		},
	}
	got := BuildOption43(menu, "192.168.1.10")
	if len(got) == 0 {
		t.Fatal("expected option 43")
	}
	if got[len(got)-1] != 0xff {
		t.Fatal("option 43 must end with ff")
	}
}

func TestSelectedType(t *testing.T) {
	got, ok := SelectedType([]byte{71, 4, 0x80, 0x02, 0x00, 0x00, 0xff})
	if !ok || got != 0x8002 {
		t.Fatalf("unexpected selection %x %v", got, ok)
	}
}

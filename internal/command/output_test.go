package command

import "testing"

func TestDecodeOutputUTF8(t *testing.T) {
	if got := DecodeOutput([]byte("已发送 = 1")); got != "已发送 = 1" {
		t.Fatalf("unexpected utf8 output %q", got)
	}
}

func TestDecodeOutputInvalidUTF8(t *testing.T) {
	got := DecodeOutput([]byte{0xff, 0xfe})
	if got == "" {
		t.Fatal("expected replacement output")
	}
}

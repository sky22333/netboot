package booturl

import (
	"testing"
)

func TestHTTPBaseOmitsDefaultPort(t *testing.T) {
	if got := HTTPBase("192.168.137.1", ":80"); got != "http://192.168.137.1" {
		t.Fatalf("HTTPBase() = %q", got)
	}
}

func TestHTTPBaseFallsBackToNextServer(t *testing.T) {
	if got := HTTPBase("", "0.0.0.0:8081"); got != "http://${next-server}:8081" {
		t.Fatalf("HTTPBase() = %q", got)
	}
}

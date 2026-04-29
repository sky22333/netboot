package booturl

import (
	"testing"

	"pxe/internal/storage"
)

func TestHTTPBootBaseUsesAdvertiseIPAndPort(t *testing.T) {
	settings := storage.ServiceSettings{}
	settings.Server.AdvertiseIP = "192.168.137.1"
	settings.HTTPBoot.Addr = ":8080"

	if got := HTTPBootBase(settings); got != "http://192.168.137.1:8080" {
		t.Fatalf("HTTPBootBase() = %q", got)
	}
}

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

func TestHTTPBaseWithListenHostPrefersConcreteHost(t *testing.T) {
	if got := HTTPBaseWithListenHost("192.168.137.1", "10.0.0.10:8080"); got != "http://10.0.0.10:8080" {
		t.Fatalf("HTTPBaseWithListenHost() = %q", got)
	}
}

package netboot

import (
	"strings"
	"testing"
)

func TestLocalVarsScriptUsesHTTPBootPort(t *testing.T) {
	script := LocalVarsScript("192.168.137.1", ":8080")
	if !strings.Contains(script, "set local-mirror http://192.168.137.1:8080") {
		t.Fatalf("expected local mirror to include custom HTTP Boot port, got:\n%s", script)
	}
}

func TestLocalVarsScriptOmitsDefaultHTTPPort(t *testing.T) {
	script := LocalVarsScript("192.168.137.1", ":80")
	if !strings.Contains(script, "set local-mirror http://192.168.137.1\n") {
		t.Fatalf("expected local mirror to omit default HTTP port, got:\n%s", script)
	}
}

func TestLocalVarsScriptFallsBackToNextServerWithPort(t *testing.T) {
	script := LocalVarsScript("", "0.0.0.0:8081")
	if !strings.Contains(script, "set local-mirror http://${next-server}:8081") {
		t.Fatalf("expected next-server fallback to include custom HTTP Boot port, got:\n%s", script)
	}
}

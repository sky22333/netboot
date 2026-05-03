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

func TestLocalVarsScriptIncludesCompatibilityGuards(t *testing.T) {
	script := LocalVarsScript("192.168.137.1", ":80")
	for _, want := range []string{
		"isset ${proxydhcp/next-server} && set use_proxydhcp_settings true ||",
		"cpuid --ext 29 && set debian_arch amd64 || set debian_arch arm64",
		"iseq ${debian_arch} amd64 && set alpine_arch x86_64 || set alpine_arch aarch64",
		"item show_info Show Boot Information",
		"echo debian_arch: ${debian_arch}",
		"echo alpine_arch: ${alpine_arch}",
		"item exit Load netboot.xyz Menu",
		"chain --autofree https://boot.netboot.xyz",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("expected generated script to contain %q, got:\n%s", want, script)
		}
	}
}

func TestLocalVarsScriptDoesNotContainBuildarch(t *testing.T) {
	script := LocalVarsScript("192.168.137.1", ":80")
	if strings.Contains(script, "buildarch") {
		t.Fatalf("expected generated script to NOT contain buildarch, got:\n%s", script)
	}
}

package logview

import (
	"strings"
	"testing"
)

func TestFormatUsesDeterministicServiceColors(t *testing.T) {
	web := Format("2026-08-06T23:40:00Z [web] INFO: ready", true)
	api := Format("2026-08-06T23:40:00Z [api] INFO: ready", true)
	if !strings.Contains(web, "m[web]\x1b[0m") || !strings.Contains(api, "m[api]\x1b[0m") {
		t.Fatalf("service tags not colored: web=%q api=%q", web, api)
	}
	if web[:strings.Index(web, "m")] == api[:strings.Index(api, "m")] {
		t.Fatalf("web and api received the same color: %q / %q", web, api)
	}
}

func TestFormatColorsFutureServicesDeterministically(t *testing.T) {
	first := Format("2026-08-06T23:40:00Z [worker] started", true)
	second := Format("2026-08-06T23:40:01Z [worker] stopped", true)
	firstColor := first[strings.Index(first, "\x1b[") : strings.Index(first, "m")+1]
	secondColor := second[strings.Index(second, "\x1b[") : strings.Index(second, "m")+1]
	if firstColor != secondColor {
		t.Fatalf("worker color changed: %q vs %q", firstColor, secondColor)
	}
	if !strings.Contains(first, "[worker]") {
		t.Fatal(first)
	}
}

func TestFormatAddsSeverityIndicators(t *testing.T) {
	cases := map[string]string{"DEBUG details": "·", "INFO: ready": "ℹ", "WARNING slow": "⚠", "ERROR: failed": "✖", "FATAL crash": "✖"}
	for input, want := range cases {
		got := Format("2026-08-06T23:40:00Z [api] "+input, false)
		if !strings.Contains(got, want+" ") {
			t.Errorf("Format(%q)=%q, want indicator %q", input, got, want)
		}
	}
}

func TestFormatPlainOutputHasNoANSI(t *testing.T) {
	got := Format("2026-08-06T23:40:00Z [web] INFO ready", false)
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("plain output contains ANSI: %q", got)
	}
	if !strings.Contains(got, "ℹ INFO ready") {
		t.Fatalf("missing readable indicator: %q", got)
	}
}

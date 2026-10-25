package main

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	withVersion := func(v string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Path: "github.com/pinebench/decrumb", Version: v}}
	}
	tests := []struct {
		name     string
		injected string
		info     *debug.BuildInfo
		want     string
	}{
		{"injected version wins", "v0.2.0", withVersion("v0.1.0"), "v0.2.0"},
		{"injected version without build info", "v0.2.0", nil, "v0.2.0"},
		{"tagged go install keeps its v", "dev", withVersion("v0.1.0"), "v0.1.0"},
		{"pseudo-version shown as is", "dev", withVersion("v0.0.0-20261019120000-abcdef123456"), "v0.0.0-20261019120000-abcdef123456"},
		{"(devel)", "dev", withVersion("(devel)"), "dev"},
		{"empty version", "dev", withVersion(""), "dev"},
		{"no build info", "dev", nil, "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVersion(tt.injected, tt.info); got != tt.want {
				t.Errorf("resolveVersion(%q, %+v) = %q, want %q", tt.injected, tt.info, got, tt.want)
			}
		})
	}
}

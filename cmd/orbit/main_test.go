package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestVersionIsTheFreeze(t *testing.T) {

	if Version != "0.1.2" {
		t.Fatalf("Version = %q, want 0.1.2", Version)
	}

	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(Version) {
		t.Fatalf("Version %q must be semver x.y.z", Version)
	}
}

func TestTitleRowsShapeAndFallback(t *testing.T) {
	if len(orbitRows) != 3 {
		t.Fatalf("title rows: %d, want 3 (the same shape as rig's titleRows)", len(orbitRows))
	}
	for i, row := range orbitRows {
		if strings.TrimSpace(row) == "" {
			t.Errorf("title row %d is empty", i)
		}
	}
	if titleName != "orbit" {
		t.Errorf("ascii fallback name = %q, want orbit", titleName)
	}
}

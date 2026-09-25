package main

import (
	"regexp"
	"testing"
)

func TestVersionIsTheFreeze(t *testing.T) {

	if Version != "0.1.1" {
		t.Fatalf("Version = %q, want 0.1.1", Version)
	}

	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(Version) {
		t.Fatalf("Version %q must be semver x.y.z", Version)
	}
}

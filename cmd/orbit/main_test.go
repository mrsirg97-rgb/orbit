package main

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestVersionStartupStaysFast(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode skips the binary build")
	}
	bin := filepath.Join(t.TempDir(), "orbit")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	best := time.Duration(0)
	for i := 0; i < 3; i++ {
		start := time.Now()
		out, err := exec.Command(bin, "-version").CombinedOutput()
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("run: %v\n%s", err, out)
		}
		if best == 0 || elapsed < best {
			best = elapsed
		}
	}
	if best > 10*time.Millisecond {
		t.Errorf("orbit -version took %s (min of 3), want < 10ms", best)
	}
}

func TestVersionIsTheFreeze(t *testing.T) {

	if Version != "0.1.5" {
		t.Fatalf("Version = %q, want 0.1.5", Version)
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

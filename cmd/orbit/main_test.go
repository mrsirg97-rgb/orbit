package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mrsirg97-rgb/rig/config"
	"github.com/mrsirg97-rgb/rig/frontend/tui"

	"github.com/mrsirg97-rgb/orbit/earn"
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

	if Version != "0.3.2" {
		t.Fatalf("Version = %q, want 0.3.2", Version)
	}

	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(Version) {
		t.Fatalf("Version %q must be semver x.y.z", Version)
	}
}

func TestStatusCallbackReflectsFireWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.json")
	fire := earn.Fire{Role: "worker", Verb: "note", Task: 4, At: time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339)}
	if err := earn.WriteSnapshot(path, earn.Rows{Held: 2, OpenClaims: 1, LastMemo: "3m ago · \"backed\"", PnLSOL: 1.5}, fire); err != nil {
		t.Fatal(err)
	}
	r := &root{earn: &earn.Command{SnapshotPath: path}}
	lines := r.earnRows(context.Background())
	if len(lines) != 4 {
		t.Fatalf("footer rows: %d, want 4:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	if !strings.Contains(lines[2], "worker note #4") || !strings.Contains(lines[2], "just now") {
		t.Errorf("the status callback must show a fire's write on the next read:\n%s", lines[2])
	}
}

func TestEarnRowsPaintLabelsEmberValuesTextAndPnLBySign(t *testing.T) {
	th, err := resolveTheme(&config.Config{}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		pnl  float64
		slot string
	}{{1.5, tui.SlotSuccess}, {-0.2, tui.SlotError}, {0, tui.SlotText}} {
		path := filepath.Join(t.TempDir(), "status.json")
		if err := earn.WriteSnapshot(path, earn.Rows{Held: 2, OpenClaims: 1, LastMemo: "3m ago · \"backed\"", PnLSOL: tc.pnl}, earn.Fire{}); err != nil {
			t.Fatal(err)
		}
		r := &root{earn: &earn.Command{SnapshotPath: path}, theme: th}
		lines := r.earnRows(context.Background())
		if len(lines) != 4 {
			t.Fatalf("footer rows: %d, want 4:\n%s", len(lines), strings.Join(lines, "\n"))
		}
		want := []string{
			th.Paint(tui.SlotEmber, "projects held: ") + th.Paint(tui.SlotText, "2"),
			th.Paint(tui.SlotEmber, "open claims: ") + th.Paint(tui.SlotText, "1"),
			th.Paint(tui.SlotEmber, "last memo: ") + th.Paint(tui.SlotText, "3m ago · \"backed\""),
			th.Paint(tui.SlotEmber, "PnL since start: ") + th.Paint(tc.slot, earn.Rows{PnLSOL: tc.pnl}.PnLText()),
		}
		for i := range want {
			if lines[i] != want[i] {
				t.Errorf("pnl %v row %d:\ngot  %q\nwant %q", tc.pnl, i, lines[i], want[i])
			}
		}
	}
}

func TestShippedThemeWhenNoHomeThemeFile(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(dir, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Theme) != 0 {
		t.Fatalf("an empty home must load no theme.json, got %s", cfg.Theme)
	}
	th, err := resolveTheme(cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	if th.Slot("ember") != "#6b7fa3" {
		t.Errorf("shipped ember = %q, want #6b7fa3", th.Slot("ember"))
	}
}

func TestHomeThemeFileOverridesShipped(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "theme.json"), []byte(`{"base":"oled","slots":{"ember":"#112233"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir, dir)
	if err != nil {
		t.Fatal(err)
	}
	th, err := resolveTheme(cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	if th.Slot("ember") != "#112233" {
		t.Errorf("home theme ember = %q, want #112233 (the home theme.json wins entirely)", th.Slot("ember"))
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

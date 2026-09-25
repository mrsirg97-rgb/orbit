package world

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// fixtureReadState is the fixed snapshot the goldens pin.
func fixtureReadState() ReadState {
	markets := []MarketView{
		{Mint: "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG", Name: "Torch Test", Symbol: "TST", Status: "BONDING", PriceSOL: 0.00015, MCAPSOL: 150000, IsHeld: true, ValueSOL: 0.1234, PnLSOL: 0.0567, Sentiment: 2.5},
		{Mint: "6wDUn9V7fuP1Ujn6o3xk4yFh4EE3F65LpgQsrNjTjmVx", Name: "Deep", Symbol: "DEEP", Status: "MIGRATED", PriceSOL: 0.00012, MCAPSOL: 120000, IsHeld: true, ValueSOL: 0.05, PnLSOL: -0.01, Sentiment: -3},
		{Mint: "9nE9pmVmGdPemmudsrZegDuMygfcTwKc95QrwSU7JUUv", Name: "Rising", Symbol: "RIS", Status: "COMPLETE", PriceSOL: 0.0003, MCAPSOL: 300000},
		{Mint: "8gsdeXDgmFG6zYhAgHgrUHM6e6rxTojFYXesL196X94d", Name: "New One", Symbol: "NEW", Status: "BONDING", PriceSOL: 0.00002, MCAPSOL: 20000},
		{Mint: "FkD69MZDUUEd5bd2eW3cjSkmc5QqVmV8SB4LrTq1ziMx", Name: "Watching", Symbol: "WAT", Status: "MIGRATED", PriceSOL: 0.001, MCAPSOL: 1000000},
		{Mint: "EwrxtuXUqFxaLMQ4uSCgyoukjC3u4UrvCVY3SRh9k1yy", Name: "Dumped", Symbol: "DMP", Status: "RECLAIMED", PriceSOL: 0, MCAPSOL: 0},
		{Mint: "75CynQP7x9quDFzxaan7bAbVYXthYNqseC63BYzsR4Qt", Name: "Alpha", Symbol: "ALP", Status: "BONDING", PriceSOL: 0.00008, MCAPSOL: 80000},
		{Mint: "3FZJJx4MSZTYpQXLyB19nRyHHCXNZyCUwW6LR97pGVmJ", Name: "Beta", Symbol: "BET", Status: "COMPLETE", PriceSOL: 0.0005, MCAPSOL: 500000},
		{Mint: "J69My5UG5ucpqqEY21Mpk523JsZ7kfaavGVJt6nBJLpE", Name: "Gamma", Symbol: "GAM", Status: "BONDING", PriceSOL: 0.00001, MCAPSOL: 10000},
		{Mint: "CQmqu26HsXBLQQuosWboQLABuojzJWRVXEVjnjCjfGQZ", Name: "Delta", Symbol: "DEL", Status: "MIGRATED", PriceSOL: 0.0004, MCAPSOL: 400000},
		{Mint: "FhY4T8MW7Ab3Di3PvomGMJhEW7EoB8C2E8KL1bNtaGT4", Name: "Echo", Symbol: "ECH", Status: "BONDING", PriceSOL: 0.00003, MCAPSOL: 30000},
		{Mint: "35j8sVeTgcNAHMEXVHptDgwyG9TkK4BxaM9gk12ByHbW", Name: "Foxtrot", Symbol: "FOX", Status: "COMPLETE", PriceSOL: 0.0002, MCAPSOL: 200000},
		{Mint: "8a3GpLLM4Wn7R1jBQX8Dtk9Tq2nVvF5YzC2HsE1uJdK9", Name: "Golf", Symbol: "GOL", Status: "BONDING", PriceSOL: 0.000015, MCAPSOL: 15000},
		{Mint: "7b2FoQnK5Xm8S2kC9R1uW0vN4tE3yJ6gZ8aLcD1fH2oP", Name: "Hotel", Symbol: "HOT", Status: "MIGRATED", PriceSOL: 0.0006, MCAPSOL: 600000},
		{Mint: "6c1GnPmJ6Wn9T3lD0S2vX1wO5uF4zK7hY9bMeE2gI3qN", Name: "India", Symbol: "IND", Status: "BONDING", PriceSOL: 0.00004, MCAPSOL: 40000},
	}
	return ReadState{
		Identity: Identity{Name: "@AP2B3A", Bio: "A torch market agent. Reads, posts, and trades with conviction."},
		PnL: PnlSummary{
			TotalRealizedPnl: 2_500_000,
			ByMint: []PnlByMint{
				{Mint: markets[0].Mint, TokensRemaining: 123_456_789, CostBasisRemaining: 9_000_000, RealizedPnl: 1_000_000},
				{Mint: markets[1].Mint, TokensRemaining: 50_000_000, CostBasisRemaining: 6_000_000, RealizedPnl: -1_500_000},
			},
		},
		Holdings: []Holding{
			{Mint: markets[0].Mint, Raw: 123_456_789, ValueSOL: 0.1234},
			{Mint: markets[1].Mint, Raw: 50_000_000, ValueSOL: 0.05},
		},
		Markets:   markets,
		Sentiment: map[string]float64{markets[0].Mint: 2.5, markets[1].Mint: -3},
		Intel: []MessageView{
			{Mint: markets[0].Mint, Sender: "So11111111111111111111111111111111111111112", Text: "backed up. strong join"},
			{Mint: markets[0].Mint, Sender: "HkPCW8mFF6ndNGzKufyfsYLcmpjTaGVKaoDAjwjBauh4", Text: "sell cut now, weak"},
			{Mint: markets[0].Mint, Sender: "2N7Wah8yDrzFEPLj7praV9kzMJau5bxwB5gaewxGYm1M", Text: "watch the treasury grow, mooning"},
			{Mint: markets[1].Mint, Sender: "So11111111111111111111111111111111111111112", Text: "dump weak scam exit"},
			{Mint: markets[1].Mint, Sender: "6bdQ7FSi3shWkzGW1Nuk6yKykqKuWxRAQwQycsk86vq2", Text: "backing the dip, strong"},
			{Mint: markets[2].Mint, Sender: "So11111111111111111111111111111111111111112", Text: "rally up, join now"},
		},
		Positions: []PositionView{{Mint: markets[1].Mint, Side: "long", Health: "at_risk", DebtSOL: 0.005}},
		VaultSOL:  10_000_000,
	}
}

func TestGoldens(t *testing.T) {
	read := fixtureReadState()
	for _, size := range []Size{Compact, Full} {
		got, err := Build(read, size)
		if err != nil {
			t.Fatal(err)
		}
		name := "compact"
		if size == Full {
			name = "full"
		}
		path := filepath.Join("testdata", name+".txt")
		if *update {
			if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s golden missing (run with -update): %v", name, err)
		}
		if got != string(want) {
			t.Errorf("%s golden differs:\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
		}
	}
}

func TestCompactTokenBand(t *testing.T) {
	got, err := Build(fixtureReadState(), Compact)
	if err != nil {
		t.Fatal(err)
	}
	n := Tokens(got)
	if n < 700 || n > 1000 {
		t.Errorf("compact tokens %d, want [700, 1000]", n)
	}
	full, err := Build(fixtureReadState(), Full)
	if err != nil {
		t.Fatal(err)
	}
	if Tokens(full) <= n {
		t.Errorf("full (%d) must exceed compact (%d)", Tokens(full), n)
	}
	if Tokens(full) <= 1300 {
		t.Errorf("full tokens %d, want > 1300", Tokens(full))
	}
}

func TestHealthNudges(t *testing.T) {
	cases := []struct {
		pnl  int64
		hold float64
		cost int64
		want string
	}{
		{2_000_000_000, 0, 0, "YOU ARE UP. consider taking profits."},
		{-2_000_000_000, 0, 0, "YOU ARE DOWN. be conservative. consider downsizing."},
		{0, 0, 0, "BREAKEVEN. look for conviction plays."},
		{0, 0.5, 500_000_000, "BREAKEVEN. look for conviction plays."},
	}
	for _, c := range cases {
		read := ReadState{
			PnL:      PnlSummary{TotalRealizedPnl: c.pnl},
			Holdings: []Holding{{Mint: "m", ValueSOL: c.hold}},
		}
		if c.cost != 0 {
			read.PnL.ByMint = []PnlByMint{{Mint: "m", CostBasisRemaining: c.cost}}
		}
		_, nudge := HealthLine(read)
		if nudge != c.want {
			t.Errorf("pnl %d hold %v cost %d: %s, want %s", c.pnl, c.hold, c.cost, nudge, c.want)
		}
	}
}

func TestProjection(t *testing.T) {
	read := fixtureReadState()
	got, err := Build(read, Compact)
	if err != nil {
		t.Fatal(err)
	}
	if statusChar("RECLAIMED") != "RAZED" {
		t.Errorf("statusChar: %s", statusChar("RECLAIMED"))
	}
	for _, want := range []string{
		"krZBG", // the 8-char FID of the fixture mint
		"jmVx",  // the second mint's FID
		"RS", "ASN",
		"T", "F",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("projection missing %q", want)
		}
	}
	if !strings.Contains(got, "@AP2B3A") {
		t.Error("identity name missing")
	}
	if !strings.Contains(got, "HLTH: +") {
		t.Error("HLTH line missing")
	}
}

func TestYouAreDescribesTheWallet(t *testing.T) {
	read := fixtureReadState()
	for _, size := range []Size{Compact, Full} {
		got, err := Build(read, size)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{
			"NAME: @AP2B3A",
			"PNL: +0.0025 SOL realized.",
			"POSITIONS: rNjTjmVx long at_risk 0.0050 SOL.",
		} {
			if !strings.Contains(got, want) {
				name := "compact"
				if size == Full {
					name = "full"
				}
				t.Errorf("%s block missing %q", name, want)
			}
		}
		for _, gone := range []string{"ROLE:", "MEMO SHAPES:", "VOICE:", "PERSONALITY:", "STAKE:"} {
			if strings.Contains(got, gone) {
				name := "compact"
				if size == Full {
					name = "full"
				}
				t.Errorf("%s block still carries %q", name, gone)
			}
		}
	}
}

func TestDeterminism(t *testing.T) {
	read := fixtureReadState()
	a, _ := Build(read, Compact)
	b, _ := Build(read, Compact)
	if a != b {
		t.Error("two builds of the same snapshot differ")
	}
}

func TestSentimentLexicon(t *testing.T) {
	cases := []struct {
		msgs []MessageView
		want float64
	}{
		{[]MessageView{{Text: "backed up. strong join"}}, 3},
		{[]MessageView{{Text: "sell cut now, weak"}}, -3},
		{[]MessageView{{Text: "back up and sell down"}}, 0},
		{nil, 0},
		{[]MessageView{{Text: "dump weak scam exit"}}, -4},
	}
	for _, c := range cases {
		got := SentimentFrom(c.msgs)
		if got != c.want {
			t.Errorf("SentimentFrom(%v) = %v, want %v", c.msgs, got, c.want)
		}
	}
}

func TestRowsAndSizes(t *testing.T) {
	read := fixtureReadState()
	compact, _ := Build(read, Compact)
	full, _ := Build(read, Full)
	if bytes.Count([]byte(compact), []byte("\n")) >= bytes.Count([]byte(full), []byte("\n")) {
		t.Error("full must have more lines than compact")
	}
	// Section set is identical: LEGEND / YOU ARE / INTEL / PROJECTS / ACTIONS /
	// RULES / STRATEGIES, both sizes.
	for _, sec := range []string{"LEGEND", "YOU ARE", "INTEL", "PROJECTS", "ACTIONS", "RULES", "STRATEGIES"} {
		if !strings.Contains(compact, sec+"\n") || !strings.Contains(full, sec+"\n") {
			t.Errorf("section %s missing in one size", sec)
		}
	}
}

// Package brief builds the agent's brief: Pyre's compact prompt shape over
// torch's read side, in torch's vocabulary. Build is a pure function of a
// ReadState snapshot — same snapshot, same bytes, which is what makes the
// goldens meaningful. The opcodes are back/exit/post/pass, the statuses are
// bonding/ready/migrated/reclaimed, and the columns are HELD/FOUNDED/
// SENTIMENT — no glyphs, no Pyre abbreviations.
package brief

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Size selects the brief.
type Size int

const (
	Compact Size = iota
	Full
)

// Identity is the agent's row: the wallet's name (@APxxxx), bio, and the
// architect's goal. The YOU ARE section describes the wallet's position
// and history — no role, no archetype.
type Identity struct {
	Name string
	Bio  string
	Goal string
}

// PnlSummary mirrors the wallet read (lamports).
type PnlSummary struct {
	TotalRealizedPnl int64
	ByMint           []PnlByMint
}

// PnlByMint is per-mint FIFO accounting.
type PnlByMint struct {
	Mint               string
	TokensRemaining    uint64
	CostBasisRemaining int64
	RealizedPnl        int64
}

// Holding is one position value (raw balance + SOL value, 6 decimals).
type Holding struct {
	Mint     string
	Raw      uint64
	ValueSOL float64
}

// MarketView is one project row for the brief.
type MarketView struct {
	Mint      string
	Name      string
	Symbol    string
	Status    string // BONDING | COMPLETE | MIGRATED | RECLAIMED
	PriceSOL  float64
	MCAPSOL   float64
	IsHeld    bool
	IsFounder bool
	ValueSOL  float64
	PnLSOL    float64
	Sentiment float64
	HasLoan   bool
}

// MessageView is one intel line.
type MessageView struct {
	Mint   string
	Sender string
	Text   string
}

// ReadState is the snapshot the tools produce.
type ReadState struct {
	Identity  Identity
	PnL       PnlSummary
	Holdings  []Holding
	Markets   []MarketView
	Sentiment map[string]float64
	Intel     []MessageView
	Positions []PositionView
	VaultSOL  uint64
}

// PositionView is one open leverage position (shown, not lent).
type PositionView struct {
	Mint    string
	Side    string
	Health  string
	DebtSOL float64
}

// Tokens is the documented 4-chars-per-token heuristic.
func Tokens(s string) int { return (utf8.RuneCountInString(s) + 3) / 4 }

// Build renders the brief. Compact ~750 tokens, full everything.
func Build(read ReadState, size Size) (string, error) {
	if read.Identity.Name == "" {
		return "", fmt.Errorf("brief: identity name required")
	}
	var b strings.Builder
	legend(&b)
	youAre(&b, read, size)
	intel(&b, read, size)
	projects(&b, read, size)
	actions(&b, size)
	rules(&b, size)
	strategies(&b, size)
	out := strings.TrimRight(b.String(), "\n") + "\n"
	return out, nil
}

func legend(b *strings.Builder) {
	b.WriteString(`LEGEND
back $ "*" — buy a project. Vault-routed: the vault pays, the memo rides the tx.
exit $ "*" — sell a project. Vault-routed: the vault receives, the memo rides the tx.
post $ "*" — post a message on a project. A micro buy carries the memo.
pass — no action this fire.
$ is one FID from PROJECTS. One action per fire. Never invent a signature.
`)
}

func youAre(b *strings.Builder, read ReadState, size Size) {
	id := read.Identity
	b.WriteString("\nYOU ARE\n")
	b.WriteString("NAME: " + id.Name + "\n")
	b.WriteString("BIO: " + id.Bio + "\n")
	if id.Goal != "" {
		b.WriteString("GOAL: " + id.Goal + "\n")
	}
	pnl, nudge := HealthLine(read)
	b.WriteString("PNL: " + pnl + ".\n")
	b.WriteString(nudge + "\n")
	b.WriteString("VAULT: " + solstr(read.VaultSOL) + " SOL.\n")
	positions(b, read)
	if size == Full {
		b.WriteString("REALIZED: " + fmtSOLSigned(lamportsToSOL(read.PnL.TotalRealizedPnl)) + " SOL.\n")
		b.WriteString("You are one contributor in a market of contributors. Every write is on-chain proof.\n")
	}
	if size == Full {
		b.WriteString("\nHOLDINGS\n")
		if len(read.Holdings) == 0 {
			b.WriteString("No holdings.\n")
		}
		for _, h := range read.Holdings {
			b.WriteString(fmt.Sprintf("%s: %s SOL (%d raw)\n", fid(h.Mint), fmtSOL(h.ValueSOL), h.Raw))
		}
	}
}

// positions renders the wallet's open leverage positions: the position,
// not a label.
func positions(b *strings.Builder, read ReadState) {
	if len(read.Positions) == 0 {
		b.WriteString("POSITIONS: none.\n")
		return
	}
	for _, p := range read.Positions {
		b.WriteString(fmt.Sprintf("POSITIONS: %s %s %s %s SOL.\n", fid(p.Mint), p.Side, p.Health, fmtSOL(p.DebtSOL)))
	}
}

// HealthLine is Pyre's rule: PnL + unrealized, then the nudge. The line is
// PNL — the wallet's overall profit and loss, not a job title.
func HealthLine(read ReadState) (string, string) {
	pnl := read.PnL.TotalRealizedPnl
	unrealized := int64(0)
	costBasis := map[string]int64{}
	for _, m := range read.PnL.ByMint {
		costBasis[m.Mint] = m.CostBasisRemaining
	}
	for _, h := range read.Holdings {
		unrealized += lamportsOf(h.ValueSOL) - costBasis[h.Mint]
	}
	hlth := pnl + unrealized
	line := fmt.Sprintf("%+.4f SOL", lamportsToSOL(hlth))
	switch {
	case lamportsToSOL(hlth) > 1:
		return line, "YOU ARE UP. consider taking profits."
	case lamportsToSOL(hlth) < -1:
		return line, "YOU ARE DOWN. be conservative. consider downsizing."
	default:
		return line, "BREAKEVEN. look for conviction plays."
	}
}

func intel(b *strings.Builder, read ReadState, size Size) {
	limit := 2
	if size == Full {
		limit = 4
	}
	mints := orderIntel(read.Intel)
	b.WriteString("\nINTEL\n")
	if len(mints) == 0 {
		b.WriteString("No recent intel.\n")
		return
	}
	for _, mint := range mints {
		if limit <= 0 {
			break
		}
		limit--
		var msgs []string
		for _, m := range read.Intel {
			if m.Mint == mint {
				msgs = append(msgs, m.Text)
			}
		}
		if size == Full {
			for i, text := range msgs {
				if i >= 3 {
					break
				}
				b.WriteString(fmt.Sprintf("%s: %s\n", fid(mint), text))
			}
		} else {
			b.WriteString(fmt.Sprintf("%s: %s\n", fid(mint), strings.Join(msgs, " | ")))
		}
	}
}

func projects(b *strings.Builder, read ReadState, size Size) {
	rows := selectRows(read, size)
	b.WriteString("\nPROJECTS\n")
	b.WriteString("FID(8) NAME STATUS MCAP PRICE HELD FOUNDED VALUE PNL SENTIMENT LOAN\n")
	b.WriteString("MCAP and PRICE are in SOL. HELD means you hold; FOUNDED means you founded.\n")
	b.WriteString("SENTIMENT is the message-board sentiment, clamped to [-10, 10].\n")
	b.WriteString("LOAN is an open leverage position on the project.\n")
	for _, m := range rows {
		b.WriteString(fmt.Sprintf("%s %s %s %s %s %s %s %s %s %s %s\n",
			fid(m.Mint), short(m.Name, 12), statusWord(m.Status),
			fmtSOL(m.MCAPSOL), fmtSOL(m.PriceSOL),
			yesno(m.IsHeld), yesno(m.IsFounder),
			fmtSOL(m.ValueSOL), fmtSOLSigned(m.PnLSOL),
			fmt.Sprintf("%.0f", m.Sentiment), yesno(m.HasLoan)))
	}
}

func actions(b *strings.Builder, size Size) {
	b.WriteString("\nACTIONS\n")
	b.WriteString("ONE ACTION PER FIRE.\n")
	b.WriteString("back $ \"*\" — buy via the vault, then reply with the tx signature + memo\n")
	b.WriteString("exit $ \"*\" — sell via the vault, then reply with the tx signature + memo\n")
	b.WriteString("post $ \"*\" — micro buy via the vault, then reply with the tx signature + memo\n")
	b.WriteString("pass — read and hold. No tool call.\n")
	b.WriteString("$ is exactly one FID from PROJECTS. The FID is the last 8 chars of the mint.\n")
	b.WriteString("Tools: market (read + act), intel (messages), wallet (pnl, positions), board (the shared board: read + act).\n")
	if size == Full {
		b.WriteString("ascend $ \"*\" — migrate a completed project (read-only gate: not wired yet)\n")
		b.WriteString("tithe $ \"*\" — harvest fees (read-only gate: not wired yet)\n")
	}
}

func rules(b *strings.Builder, size Size) {
	b.WriteString("\nRULES\n")
	b.WriteString("One action per fire.\n")
	b.WriteString("Only touch projects in PROJECTS.\n")
	b.WriteString("Writes are vault-routed: the agent hot wallet signs, the operator's key never enters the process.\n")
	b.WriteString("Every write reply carries the tx signature plus the memo.\n")
	b.WriteString("When PNL says down, downsize: prefer exit over back.\n")
	b.WriteString("When PNL says up, take profits: prefer exit on the biggest winner.\n")
	b.WriteString("Read the project before the action: market first, intel second, wallet third.\n")
	b.WriteString("A memo without a reason is noise; cite a number.\n")
	b.WriteString("Never spend below the vault's rent floor; a failed tx is a failed fire.\n")
	if size == Full {
		b.WriteString("A post is a micro buy: the indexer persists memos only on torch txs.\n")
		b.WriteString("Never fabricate a signature; a failed tx is a failed fire.\n")
	}
}

func strategies(b *strings.Builder, size Size) {
	b.WriteString("\nSTRATEGIES\n")
	b.WriteString("Read before you act: market first, then intel, then wallet.\n")
	b.WriteString("Buy early: back small projects early; price follows volume.\n")
	b.WriteString("Cut losers: exit a project that went below your cost basis and stays bearish.\n")
	b.WriteString("Post with conviction: post only when you have a reason other contributors can verify.\n")
	b.WriteString("Follow the treasury: projects whose treasury SOL grows are accumulating.\n")
	b.WriteString("Respect sentiment: SENTIMENT below -2 on a held project is an exit signal.\n")
	b.WriteString("Never chase: no back above 2x your cost basis.\n")
	b.WriteString("One position per project: back only if you hold nothing or the thesis changed.\n")
	b.WriteString("Prefer the strongest: if two projects compete, back the one more contributors hold.\n")
	b.WriteString("Intensity: a project that just got a back from another contributor is worth watching.\n")
	b.WriteString("Treasury growth is accumulation: projects whose treasury SOL grows are building.\n")
	b.WriteString("Sentiment flips fast: an exit signal on a held project is a reason to exit first.\n")
	b.WriteString("Patience beats noise: no action is an action. Pass when the read is unclear.\n")
	if size == Full {
		b.WriteString("Diversify: no project above 50%% of the vault's value.\n")
		b.WriteString("Escalate: a project at migrated with a deep pool is a liquidity story, not a pump.\n")
		b.WriteString("Use intel: another contributor's back on your project is bullish; their exit is bearish.\n")
		b.WriteString("Be a legend: leave one memorable one-liner per fire when you post.\n")
		b.WriteString("Protect the vault: never spend below the rent floor of vault SOL.\n")
		b.WriteString("Read the leaderboard: the top MCAP project is the market's consensus.\n")
		b.WriteString("A project at ready with a growing treasury is a launch candidate.\n")
		b.WriteString("Watch the intel: three bearish memos on a held project is an exit.\n")
		b.WriteString("Reply with proof: every action ends with the tx signature and the memo.\n")
		b.WriteString("Never trust a memo without a tx: proof is the signature and the memo.\n")
		b.WriteString("When in doubt, read the treasury and the last five trades before acting.\n")
		b.WriteString("The market rewards consistency: a steady agent is a credible agent.\n")
	}
}

func selectRows(read ReadState, size Size) []MarketView {
	held := []MarketView{}
	other := []MarketView{}
	for _, m := range read.Markets {
		if m.IsHeld {
			held = append(held, m)
		} else {
			other = append(other, m)
		}
	}
	sort.SliceStable(held, func(i, j int) bool { return held[i].ValueSOL > held[j].ValueSOL })
	sort.SliceStable(other, func(i, j int) bool { return other[i].MCAPSOL > other[j].MCAPSOL })
	heldCount := 5
	total := 8
	if size == Full {
		heldCount = 10
		total = 15
	}
	if len(held) > heldCount {
		held = held[:heldCount]
	}
	room := total - len(held)
	if room < 0 {
		room = 0
	}
	if len(other) > room {
		other = other[:room]
	}
	return append(held, other...)
}

func orderIntel(msgs []MessageView) []string {
	seen := map[string]bool{}
	var order []string
	for _, m := range msgs {
		if !seen[m.Mint] {
			seen[m.Mint] = true
			order = append(order, m.Mint)
		}
	}
	return order
}

func fid(mint string) string {
	if len(mint) <= 8 {
		return mint
	}
	return mint[len(mint)-8:]
}

func short(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// statusWord is torch's status vocabulary: bonding, ready, migrated,
// reclaimed. No Pyre abbreviations.
func statusWord(status string) string {
	switch status {
	case "BONDING":
		return "bonding"
	case "COMPLETE":
		return "ready"
	case "MIGRATED":
		return "migrated"
	case "RECLAIMED":
		return "reclaimed"
	default:
		return "?"
	}
}

func yesno(b bool) string {
	if b {
		return "T"
	}
	return "F"
}

func fmtSOL(v float64) string {
	switch {
	case v >= 1:
		return fmt.Sprintf("%.2f", v)
	case v >= 0.001:
		return fmt.Sprintf("%.4f", v)
	case v == 0:
		return "0.0000"
	default:
		return fmt.Sprintf("%.6f", v)
	}
}

func fmtSOLSigned(v float64) string {
	if v >= 0 {
		return "+" + fmtSOL(v)
	}
	return "-" + fmtSOL(-v)
}

func lamportsToSOL(l int64) float64 {
	if l >= 0 {
		return float64(l) / 1e9
	}
	return -float64(-l) / 1e9
}

func lamportsOf(v float64) int64 {
	if v >= 0 {
		return int64(v * 1e9)
	}
	return -int64(-v * 1e9)
}

// bullWords / bearWords are the deterministic sentiment lexicon.
var bullWords = map[string]bool{
	"back": true, "backed": true, "backing": true,
	"buy": true, "buying": true, "bought": true,
	"up": true, "in": true, "join": true, "joined": true, "hold": true, "holding": true,
	"strong": true, "moon": true, "mooning": true, "rally": true, "win": true,
	"love": true, "bullish": true, "bull": true,
}
var bearWords = map[string]bool{
	"cut": true, "cutting": true, "sell": true, "selling": true, "sold": true,
	"out": true, "down": true, "exit": true, "dump": true, "dumped": true, "dumping": true,
	"weak": true, "rug": true, "rugpull": true, "scam": true, "bearish": true,
	"lose": true, "losing": true, "trash": true, "trashing": true,
}

// SentimentFrom scores a message board: +1 per bullish word, -1 per bearish
// word, normalized by message count, clamped to [-10, 10]. Same messages,
// same number — no LLM, no state.
func SentimentFrom(msgs []MessageView) float64 {
	if len(msgs) == 0 {
		return 0
	}
	score := 0
	for _, m := range msgs {
		for _, w := range strings.Fields(strings.ToLower(m.Text)) {
			if bullWords[w] {
				score++
			}
			if bearWords[w] {
				score--
			}
		}
	}
	norm := float64(score) / float64(len(msgs))
	if norm > 10 {
		norm = 10
	}
	if norm < -10 {
		norm = -10
	}
	return norm
}

func solstr(lamports uint64) string {
	return fmtSOL(float64(lamports) / 1e9)
}

// Bull reports whether the word is in the bullish lexicon (debug/testing).
func Bull(w string) bool { return bullWords[w] }

// Bear reports whether the word is in the bearish lexicon (debug/testing).
func Bear(w string) bool { return bearWords[w] }

// StubBrief is the prompt stored at register/refresh: the identity header
// and the static rules, with a line telling the fire to rebuild the live
// brief. The real brief is built per fire by run-job.
func StubBrief(id Identity) string {
	b := &strings.Builder{}
	b.WriteString("LEGEND\nback $ \"*\" — buy a project. Vault-routed: the vault pays, the memo rides the tx.\n")
	b.WriteString("exit $ \"*\" — sell a project. Vault-routed: the vault receives, the memo rides the tx.\n")
	b.WriteString("post $ \"*\" — post a message on a project. A micro buy carries the memo.\n")
	b.WriteString("pass — no action this fire.\n")
	b.WriteString("$ is one FID from PROJECTS. One action per fire. Never invent a signature.\n")
	b.WriteString("\nYOU ARE\n")
	b.WriteString("NAME: " + id.Name + "\n")
	b.WriteString("BIO: " + id.Bio + "\n")
	if id.Goal != "" {
		b.WriteString("GOAL: " + id.Goal + "\n")
	}
	b.WriteString("\nTHIS FIRE REBUILDS THE BRIEF: run-job snapshots the live read side\n")
	b.WriteString("and replaces this stub with the current brief (PROJECTS, INTEL, PNL).\n")
	b.WriteString("Read the live state with the market/intel/wallet tools.\n")
	return b.String()
}

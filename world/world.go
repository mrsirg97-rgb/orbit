// Package world builds the agent's brief: Pyre's compact prompt shape over
// torch's read side. Build is a pure function of a ReadState snapshot — same
// snapshot, same bytes, which is what makes the goldens meaningful.
package world

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Size selects the block.
type Size int

const (
	Compact Size = iota
	Full
)

// Identity is the agent's row: name (@APxxxx), bio, personality.
type Identity struct {
	Name        string
	Bio         string
	Personality string
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

// MarketView is one project row for the block.
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

// Build renders the block. Compact ~850 tokens, full everything.
func Build(read ReadState, size Size) (string, error) {
	if read.Identity.Name == "" {
		return "", fmt.Errorf("world: identity name required")
	}
	var b strings.Builder
	legend(&b)
	youAre(&b, read, size)
	intel(&b, read, size)
	projects(&b, read, size)
	actions(&b, size)
	rules(&b, size)
	strategies(&b, read, size)
	out := strings.TrimRight(b.String(), "\n") + "\n"
	return out, nil
}

func legend(b *strings.Builder) {
	b.WriteString(`LEGEND
(&) $ "*" → BACK — buy a project. Vault-routed: the vault pays, the memo rides the tx.
(-) $ "*" → CUT — sell a project. Vault-routed: the vault receives, the memo rides the tx.
(!) $ "*" → MEMO — post a message on a project. A micro buy carries the memo.
(_) → SKIP — no action this fire.
$ is one FID from PROJECTS. One action per fire. Never invent a signature.
`)
}

func youAre(b *strings.Builder, read ReadState, size Size) {
	id := read.Identity
	b.WriteString("\nYOU ARE\n")
	b.WriteString("NAME: " + id.Name + "\n")
	b.WriteString("BIO: " + id.Bio + "\n")
	b.WriteString("PERSONALITY: " + id.Personality + "\n")
	hlth, nudge := HealthLine(read)
	b.WriteString("HLTH: " + hlth + ".\n")
	b.WriteString(nudge + "\n")
	b.WriteString("VAULT: " + solstr(read.VaultSOL) + " SOL.\n")
	b.WriteString("You are one agent in a market of agents. Every write is on-chain proof.\n")
	if size == Full {
		b.WriteString("VOICE: " + voiceFor(id.Personality) + "\n")
		b.WriteString("\nHOLDINGS\n")
		if len(read.Holdings) == 0 {
			b.WriteString("No holdings.\n")
		}
		for _, h := range read.Holdings {
			b.WriteString(fmt.Sprintf("%s: %s SOL (%d raw)\n", fid(h.Mint), fmtSOL(h.ValueSOL), h.Raw))
		}
	}
}

// HealthLine is Pyre's rule: PnL + unrealized, then the nudge.
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
	b.WriteString("FID(8) NAME STATUS MCAP PRICE MBR FNR VALUE PNL SENT LOAN\n")
	b.WriteString("MCAP and PRICE are in SOL. MBR means you hold; FNR means you founded.\n")
	b.WriteString("SENT is the message-board sentiment, clamped to [-10, 10].\n")
	b.WriteString("LOAN is an open leverage position on the project.\n")
	for _, m := range rows {
		b.WriteString(fmt.Sprintf("%s %s %s %s %s %s %s %s %s %s %s\n",
			fid(m.Mint), short(m.Name, 12), statusChar(m.Status),
			fmtSOL(m.MCAPSOL), fmtSOL(m.PriceSOL),
			yesno(m.IsHeld), yesno(m.IsFounder),
			fmtSOL(m.ValueSOL), fmtSOLSigned(m.PnLSOL),
			fmt.Sprintf("%.0f", m.Sentiment), yesno(m.HasLoan)))
	}
}

func actions(b *strings.Builder, size Size) {
	b.WriteString("\nACTIONS\n")
	b.WriteString("ONE ACTION PER FIRE.\n")
	b.WriteString("(&) $ \"*\" → BACK — buy via the vault, then reply with the tx signature + memo\n")
	b.WriteString("(-) $ \"*\" → CUT — sell via the vault, then reply with the tx signature + memo\n")
	b.WriteString("(!) $ \"*\" → MEMO — micro buy via the vault, then reply with the tx signature + memo\n")
	b.WriteString("(_) → SKIP — read and hold. No tool call.\n")
	b.WriteString("$ is exactly one FID from PROJECTS. The FID is the last 8 chars of the mint.\n")
	b.WriteString("Tools: market (read + act), intel (messages), wallet (pnl, positions, health).\n")
	if size == Full {
		b.WriteString("(^) $ \"*\" → ASCEND — migrate a completed project (read-only gate: not wired yet)\n")
		b.WriteString("(~) $ \"*\" → TITHE — harvest fees (read-only gate: not wired yet)\n")
	}
}

func rules(b *strings.Builder, size Size) {
	b.WriteString("\nRULES\n")
	b.WriteString("One action per fire.\n")
	b.WriteString("Only touch projects in PROJECTS.\n")
	b.WriteString("Writes are vault-routed: the agent hot wallet signs, the operator's key never enters the process.\n")
	b.WriteString("Every write reply carries the tx signature plus the memo.\n")
	b.WriteString("The vault is the wallet: buy spends vault SOL, sell credits vault SOL.\n")
	b.WriteString("When HLTH says down, downsize: prefer CUT over BACK.\n")
	b.WriteString("When HLTH says up, take profits: prefer CUT on the biggest winner.\n")
	b.WriteString("Read the project before the action: market first, intel second, wallet third.\n")
	b.WriteString("The memo is the voice. A memo without a reason is noise; cite a number.\n")
	b.WriteString("Never spend below the vault's rent floor; a failed tx is a failed fire.\n")
	if size == Full {
		b.WriteString("Memo is a micro buy: the indexer persists memos only on torch txs.\n")
		b.WriteString("Never fabricate a signature; a failed tx is a failed fire.\n")
	}
}

func strategies(b *strings.Builder, read ReadState, size Size) {
	b.WriteString("\nSTRATEGIES\n")
	b.WriteString("Read before you act: market first, then intel, then wallet.\n")
	b.WriteString("Buy early: BACK small projects early; price follows volume.\n")
	b.WriteString("Cut losers: CUT a project that went below your cost basis and stays bearish.\n")
	b.WriteString("Post with conviction: MEMO only when you have a reason an ally can verify.\n")
	b.WriteString("Follow the treasury: projects whose treasury SOL grows are accumulating.\n")
	b.WriteString("Respect sentiment: SENT below -2 on a held project is a CUT signal.\n")
	b.WriteString("Never chase: no BACK above 2x your cost basis.\n")
	b.WriteString("One position per project: BACK only if you hold nothing or the thesis changed.\n")
	b.WriteString("Prefer the strongest: if two projects compete, back the one with more MBR.\n")
	b.WriteString("Intensity: a project that just got a BACK from a rival is worth watching.\n")
	b.WriteString("Treasury growth is accumulation: projects whose treasury SOL grows are building.\n")
	b.WriteString("Sentiment flips fast: a CUT signal on a held project is a reason to cut first.\n")
	b.WriteString("Patience beats noise: no action is an action. Skip when the read is unclear.\n")
	if size == Full {
		b.WriteString("Diversify: no project above 50%% of the vault's value.\n")
		b.WriteString("Escalate: a project at ASN with a deep pool is a liquidity story, not a pump.\n")
		b.WriteString("Use intel: a rival's BACK on your project is bullish; a rival's CUT is bearish.\n")
		b.WriteString("Be a legend: leave one memorable one-liner per fire when you MEMO.\n")
		b.WriteString("Protect the vault: never spend below the rent floor of vault SOL.\n")
		b.WriteString("Read the leaderboard: the top MCAP project is the market's consensus.\n")
		b.WriteString("A project at RD with a growing treasury is a launch candidate.\n")
		b.WriteString("Watch the intel: three bearish memos on a held project is a CUT.\n")
		b.WriteString("Reply with proof: every action ends with the tx signature and the memo.\n")
		b.WriteString("Stay in character: your personality colors every memo and every choice.\n")
		b.WriteString("Never trust a memo without a tx: proof is the signature and the memo.\n")
		b.WriteString("When in doubt, read the treasury and the last five trades before acting.\n")
		b.WriteString("The market rewards consistency: a steady agent is a credible agent.\n")
	}
}

func voiceFor(personality string) string {
	switch personality {
	case "loyalist":
		return "Ride or die. Trash talk rivals by address. Hype your crew loudly."
	case "mercenary":
		return "Lone wolf. Every angle is a trade; drop alpha only when it pays."
	case "provocateur":
		return "Chaos and hot takes. Call out the biggest holder. Make bets."
	case "scout":
		return "The intel operative. Share data that makes people nervous."
	case "whale":
		return "You move markets. Flex size. Back words with big moves."
	default:
		return "Short, punchy. One action per fire."
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

func statusChar(status string) string {
	switch status {
	case "BONDING":
		return "RS"
	case "COMPLETE":
		return "RD"
	case "MIGRATED":
		return "ASN"
	case "RECLAIMED":
		return "RAZED"
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

// StubBlock is the prompt stored at register/refresh: the identity header and
// the static rules, with a line telling the fire to rebuild the live block.
// The real world block is built per fire by run-job.
func StubBlock(id Identity) string {
	b := &strings.Builder{}
	b.WriteString("LEGEND\n(&) $ \"*\" → BACK — buy a project. Vault-routed: the vault pays, the memo rides the tx.\n")
	b.WriteString("(-) $ \"*\" → CUT — sell a project. Vault-routed: the vault receives, the memo rides the tx.\n")
	b.WriteString("(!) $ \"*\" → MEMO — post a message on a project. A micro buy carries the memo.\n")
	b.WriteString("(_) → SKIP — no action this fire.\n")
	b.WriteString("$ is one FID from PROJECTS. One action per fire. Never invent a signature.\n")
	b.WriteString("\nYOU ARE\n")
	b.WriteString("NAME: " + id.Name + "\n")
	b.WriteString("BIO: " + id.Bio + "\n")
	b.WriteString("PERSONALITY: " + id.Personality + "\n")
	b.WriteString("\nTHIS FIRE REBUILDS THE WORLD BLOCK: run-job snapshots the live read side\n")
	b.WriteString("and replaces this stub with the current block (PROJECTS, INTEL, HLTH).\n")
	b.WriteString("Read the live state with the market/intel/wallet tools.\n")
	return b.String()
}

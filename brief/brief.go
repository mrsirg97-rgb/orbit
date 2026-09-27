package brief

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

type Size int

const (
	Compact Size = iota
	Full
)

type Identity struct {
	Name string
	Bio  string
	Goal string
}

type PnlSummary struct {
	TotalRealizedPnl int64
	ByMint           []PnlByMint
}

type PnlByMint struct {
	Mint               string
	TokensRemaining    uint64
	CostBasisRemaining int64
	RealizedPnl        int64
}

type Holding struct {
	Mint     string
	Raw      uint64
	ValueSOL float64
}

type MarketView struct {
	Mint        string
	Name        string
	Symbol      string
	Status      string
	PriceSOL    float64
	MCAPSOL     float64
	IsHeld      bool
	IsFounder   bool
	ValueSOL    float64
	PnLSOL      float64
	Sentiment   float64
	HasLoan     bool
	TreasurySOL float64
	Lending     bool
}

type MessageView struct {
	Mint   string
	Sender string
	Text   string
}

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

type PositionView struct {
	Mint    string
	Side    string
	Health  string
	DebtSOL float64
}

func Tokens(s string) int { return (utf8.RuneCountInString(s) + 3) / 4 }

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
The town is a gig economy; the acts are orbit's words over torch's market.
invest $ "*" — capital for exposure, no task. Any state but closed.
contract $ N "*" — pick up task N with your own capital. Public only.
work $ N "*" — pick up task N on the treasury's leverage against your holding. Public and lending.
release $ [N] — let go: sell the investment, or with N free task N and close its commitment.
short $ [N "*"] — dissent with capital; with N it rejects a completion.
post $ "*" — speech with stake: a micro buy carries the memo.
pass — no action this fire.
STATE: private=bonding, funded=ready, public=migrated, closed=reclaimed (torch's names).
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
	b.WriteString("EARNINGS: " + pnl + ".\n")
	b.WriteString(nudge + "\n")
	b.WriteString("VAULT: " + solstr(read.VaultSOL) + " SOL.\n")
	positions(b, read)
	if size == Full {
		b.WriteString("REALIZED: " + fmtSOLSigned(lamportsToSOL(read.PnL.TotalRealizedPnl)) + " SOL.\n")
		b.WriteString("You are a freelancer in a busy town. Skill gets you gigs, reputation gets you better ones, and the biggest communities pay best. Every write is on-chain proof.\n")
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

func positions(b *strings.Builder, read ReadState) {
	if len(read.Positions) == 0 {
		b.WriteString("COMMITMENTS: none.\n")
		return
	}
	for _, p := range read.Positions {
		b.WriteString(fmt.Sprintf("COMMITMENTS: %s %s %s %s SOL.\n", fid(p.Mint), p.Side, p.Health, fmtSOL(p.DebtSOL)))
	}
}

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
	b.WriteString("\nGOSSIP\n")
	if len(mints) == 0 {
		b.WriteString("Nothing on the board lately.\n")
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
	b.WriteString("FID(8) NAME STATE SIZE RATE HELD FOUNDED VALUE EARN GOSSIP COMMIT BACKING LENDS\n")
	b.WriteString("SIZE, RATE, VALUE, EARN, BACKING in SOL. GOSSIP is the board's mood in [-10, 10]. COMMIT is an open commitment.\n")
	b.WriteString("LENDS F on a public project means public, not lending yet: contract, not work.\n")
	for _, m := range rows {
		b.WriteString(fmt.Sprintf("%s %s %s %s %s %s %s %s %s %s %s %s %s\n",
			fid(m.Mint), short(m.Name, 12), StateWord(m.Status),
			fmtSOL(m.MCAPSOL), fmtSOL(m.PriceSOL),
			yesno(m.IsHeld), yesno(m.IsFounder),
			fmtSOL(m.ValueSOL), fmtSOLSigned(m.PnLSOL),
			fmt.Sprintf("%.0f", m.Sentiment), yesno(m.HasLoan),
			fmtSOL(m.TreasurySOL), yesno(m.Lending)))
	}
}

func actions(b *strings.Builder, size Size) {
	b.WriteString("\nACTIONS\n")
	b.WriteString("ONE ACTION PER FIRE. The acts are in LEGEND; every write is vault-routed and replies with the tx signature + memo.\n")
	if size == Full {
		b.WriteString("invest $ \"*\" — buy via the vault\n")
		b.WriteString("contract $ N \"*\" — buy above the memo stake with claim N\n")
		b.WriteString("work $ N \"*\" — open a long on your holding with claim N\n")
		b.WriteString("release $ [N] — sell, or close the commitment and free task N\n")
		b.WriteString("short $ [N \"*\"] — open a short, with reject N when it answers a completion\n")
		b.WriteString("post $ \"*\" — micro buy via the vault\n")
	}
	b.WriteString("pass — read and hold. No tool call.\n")
	b.WriteString("$ is exactly one FID from PROJECTS. The FID is the last 8 chars of the mint.\n")
	b.WriteString("Tools: projects (list, show), market (read + the acts), intel (messages), wallet (earnings, commitments, reputation), board (the shared board).\n")
	if size == Full {
		b.WriteString("A contract is your own capital and runs no clock. Work is the treasury's leverage on your holding: interest runs until you release, and a sinking rate can wash you out.\n")
	}
}

func rules(b *strings.Builder, size Size) {
	b.WriteString("\nRULES\n")
	b.WriteString("One action per fire. Only touch projects in PROJECTS.\n")
	b.WriteString("Private projects take invest and post, never a task or a claim: fund it first.\n")
	b.WriteString("Work is borrowed budget with a clock on it; getting washed out costs the stake. Read your standing before you size a claim.\n")
	b.WriteString("Only the funder's accept counts; the rate is the judge of whether the work landed.\n")
	b.WriteString("When EARNINGS say down, downsize: prefer release over invest.\n")
	b.WriteString("Read the project before the act: market first, intel second, wallet third.\n")
	b.WriteString("A memo without a reason is noise; cite a number.\n")
	b.WriteString("Never spend below the vault's rent floor; a failed tx is a failed fire.\n")
	if size == Full {
		b.WriteString("Writes are vault-routed: the agent hot wallet signs, the operator's key never enters the process.\n")
		b.WriteString("A post is a micro buy: the indexer persists memos only on torch txs.\n")
		b.WriteString("Never fabricate a signature; a failed tx is a failed fire.\n")
		b.WriteString("A reviewer who stalls costs a worker interest; take a contract when review may be slow.\n")
	}
}

func strategies(b *strings.Builder, size Size) {
	b.WriteString("\nSTRATEGIES\n")
	b.WriteString("Back the round early: invest in small private projects; the rate follows the members.\n")
	b.WriteString("Take gigs where the backing is: LENDS T funds work; LENDS F takes contracts.\n")
	b.WriteString("Cut losers: release an investment below your cost basis that stays bearish.\n")
	b.WriteString("Post with conviction: only a reason other members can verify.\n")
	b.WriteString("Respect gossip: GOSSIP below -2 on a held project is a release signal.\n")
	b.WriteString("One commitment per project: work only if you hold nothing on leverage there.\n")
	b.WriteString("Dissent pays or costs: short a completion you can disprove, and cite the number.\n")
	b.WriteString("Patience beats noise: no action is an action. Pass when the read is unclear.\n")
	if size == Full {
		b.WriteString("Read before you act: market first, then intel, then wallet.\n")
		b.WriteString("Follow the backing: projects whose treasury grows are accumulating.\n")
		b.WriteString("Never chase: no invest above 2x your cost basis.\n")
		b.WriteString("Diversify: no project above 50% of the vault's value.\n")
		b.WriteString("Escalate: a public project with a deep pool is a liquidity story, not a pump.\n")
		b.WriteString("Use intel: another member's invest on your project is bullish; their release is bearish.\n")
		b.WriteString("Be a legend: leave one memorable one-liner per fire when you post.\n")
		b.WriteString("Protect the vault: never spend below the rent floor of vault SOL.\n")
		b.WriteString("Read the rankings: the largest community is the town's consensus; town GDP is every project summed.\n")
		b.WriteString("A funded project with a growing treasury is a launch candidate.\n")
		b.WriteString("Watch the gossip: three bearish memos on a held project is a release.\n")
		b.WriteString("Reputation is the ledger: released at a surplus, accepts received, washed out. Anyone can recompute yours.\n")
		b.WriteString("Reply with proof: every action ends with the tx signature and the memo.\n")
		b.WriteString("Never trust a memo without a tx: proof is the signature and the memo.\n")
		b.WriteString("When in doubt, read the treasury and the last five trades before acting.\n")
		b.WriteString("The town rewards consistency: a steady agent is a credible agent.\n")
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
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func StateWord(status string) string {
	switch status {
	case "BONDING":
		return "private"
	case "COMPLETE":
		return "funded"
	case "MIGRATED":
		return "public"
	case "RECLAIMED":
		return "closed"
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

func Bull(w string) bool { return bullWords[w] }

func Bear(w string) bool { return bearWords[w] }

func StubBrief(id Identity) string {
	b := &strings.Builder{}
	legend(b)
	b.WriteString("\nYOU ARE\n")
	b.WriteString("NAME: " + id.Name + "\n")
	b.WriteString("BIO: " + id.Bio + "\n")
	if id.Goal != "" {
		b.WriteString("GOAL: " + id.Goal + "\n")
	}
	b.WriteString("\nTHIS FIRE REBUILDS THE BRIEF: run-job snapshots the live read side\n")
	b.WriteString("and replaces this stub with the current brief (PROJECTS, GOSSIP, EARNINGS).\n")
	b.WriteString("Read the live state with the market/intel/wallet tools.\n")
	return b.String()
}

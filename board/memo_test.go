package board

import (
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/idl"
)

func TestMemoShapesRoundTrip(t *testing.T) {
	cases := []Shape{
		{Role: "architect", Verb: "task", ID: 3, Text: "Measure the compact size"},
		{Role: "architect", Verb: "brief", ID: 3, Text: "One paragraph of context"},
		{Role: "worker", Verb: "claim", ID: 3},
		{Role: "worker", Verb: "note", ID: 3, Text: "42 fires folded"},
		{Role: "worker", Verb: "complete", ID: 3},
		{Role: "reviewer", Verb: "accept", ID: 3},
		{Role: "architect", Verb: "accept", ID: 3},
		{Role: "reviewer", Verb: "reject", ID: 3, Text: "No proof in the memo"},
	}
	want := []string{
		"[architect] task 3: Measure the compact size",
		"[architect] brief 3: One paragraph of context",
		"[worker] claim 3",
		"[worker] note 3: 42 fires folded",
		"[worker] complete 3",
		"[reviewer] accept 3",
		"[architect] accept 3",
		"[reviewer] reject 3: No proof in the memo",
	}
	for i, c := range cases {
		memo, err := MemoFor(c)
		if err != nil {
			t.Fatalf("%s: %v", c.Verb, err)
		}
		if memo != want[i] {
			t.Errorf("%s: memo %q, want %q", c.Verb, memo, want[i])
		}
		parsed, ok := ParseMemo(memo)
		if !ok {
			t.Fatalf("%s: parse failed", c.Verb)
		}
		if parsed.Role != c.Role || parsed.Verb != c.Verb || parsed.ID != c.ID || parsed.Text != c.Text {
			t.Errorf("%s: parsed %+v, want %+v", c.Verb, parsed, c)
		}
	}
}

func TestMemoShapesRoleGate(t *testing.T) {
	if _, err := MemoFor(Shape{Role: "worker", Verb: "task", ID: 1, Text: "x"}); err == nil {
		t.Error("worker task accepted")
	}
	if _, err := MemoFor(Shape{Role: "architect", Verb: "claim", ID: 1}); err == nil {
		t.Error("architect claim accepted")
	}
	if _, err := MemoFor(Shape{Role: "worker", Verb: "reject", ID: 1, Text: "x"}); err == nil {
		t.Error("worker reject accepted")
	}
}

func TestMemoShapesMalformedSkipped(t *testing.T) {
	for _, bad := range []string{
		"task 3: no tag",
		"[worker] task 3: wrong role",
		"[architect] task 0: zero id",
		"[architect] task x: bad id",
		"[worker] claim 3 extra",
		"[architect] goal: not a board verb",
		"garbage",
	} {
		if _, ok := ParseMemo(bad); ok {
			t.Errorf("malformed memo parsed: %q", bad)
		}
	}
}

func TestMemoShapesAgainstIDL(t *testing.T) {
	id, err := idl.LoadIDL()
	if err != nil {
		t.Fatal(err)
	}
	disc, err := id.Discriminator("buy_via_vault")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []Shape{
		{Role: "architect", Verb: "task", ID: 1, Text: "Context compaction"},
		{Role: "worker", Verb: "claim", ID: 1},
		{Role: "reviewer", Verb: "reject", ID: 1, Text: "No proof"},
	} {
		memo, err := MemoFor(c)
		if err != nil {
			t.Fatalf("%s: %v", c.Verb, err)
		}
		if utf8.RuneCountInString(memo) > MemoCap {
			t.Errorf("%s: memo %d chars, cap %d", c.Verb, utf8.RuneCountInString(memo), MemoCap)
		}
		args, err := id.BorshArgs("buy_via_vault", map[string]any{
			"sol_amount":     uint64(client.MemoBuyLamports),
			"min_tokens_out": uint64(9_900_000),
		})
		if err != nil {
			t.Fatal(err)
		}
		data := append(append([]byte{}, disc...), args...)
		if len(data) != 8+16 {
			t.Fatalf("buy data: %d bytes", len(data))
		}
		if got := binary.LittleEndian.Uint64(data[8:16]); got != uint64(client.MemoBuyLamports) {
			t.Errorf("sol_amount: %d", got)
		}
		if got := binary.LittleEndian.Uint64(data[16:24]); got != 9_900_000 {
			t.Errorf("min_tokens_out: %d", got)
		}
		// The memo rides the same tx, co-resident with the torch instruction.
		memoIx, err := client.BuildMemo("signer", memo)
		if err != nil {
			t.Fatal(err)
		}
		if string(memoIx.Data) != memo {
			t.Errorf("memo bytes: %q", string(memoIx.Data))
		}
		if hex.EncodeToString(data[:8]) != hex.EncodeToString(disc) {
			t.Errorf("discriminator: %x", data[:8])
		}
	}
}

func TestMemoCapRefusesOverlong(t *testing.T) {
	long := strings.Repeat("x", MemoCap)
	if _, err := MemoFor(Shape{Role: "architect", Verb: "task", ID: 1, Text: long}); err == nil {
		t.Error("overlong memo accepted")
	}
	if _, err := MemoFor(Shape{Role: "architect", Verb: "task", ID: 1, Text: strings.Repeat("x", MemoCap-20)}); err != nil {
		t.Errorf("cap-bound memo refused: %v", err)
	}
}

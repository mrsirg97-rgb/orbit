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
		{Verb: "task", ID: 3, Text: "Measure the compact size"},
		{Verb: "brief", ID: 3, Text: "One paragraph of context"},
		{Verb: "claim", ID: 3},
		{Verb: "note", ID: 3, Text: "42 fires folded"},
		{Verb: "complete", ID: 3},
		{Verb: "accept", ID: 3},
		{Verb: "reject", ID: 3, Text: "No proof in the memo"},
	}
	want := []string{
		"task 3: Measure the compact size",
		"brief 3: One paragraph of context",
		"claim 3",
		"note 3: 42 fires folded",
		"complete 3",
		"accept 3",
		"reject 3: No proof in the memo",
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
		if parsed.Verb != c.Verb || parsed.ID != c.ID || parsed.Text != c.Text {
			t.Errorf("%s: parsed %+v, want %+v", c.Verb, parsed, c)
		}
	}
}

func TestMemoShapesCarryNoRoleTag(t *testing.T) {
	for _, c := range []Shape{
		{Verb: "task", ID: 3, Text: "x"},
		{Verb: "claim", ID: 3},
		{Verb: "reject", ID: 3, Text: "x"},
	} {
		memo, err := MemoFor(c)
		if err != nil {
			t.Fatalf("%s: %v", c.Verb, err)
		}
		if strings.HasPrefix(memo, "[") {
			t.Errorf("%s memo still carries a tag: %q", c.Verb, memo)
		}
	}
}

func TestMemoShapesMalformedSkipped(t *testing.T) {
	for _, bad := range []string{
		"[worker] task 3: wrong role",
		"[architect] claim 3",
		"task 0: zero id",
		"task x: bad id",
		"claim 3 extra",
		"goal: not a board verb",
		"garbage",
		"note: no id",
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
		{Verb: "task", ID: 1, Text: "Context compaction"},
		{Verb: "claim", ID: 1},
		{Verb: "reject", ID: 1, Text: "No proof"},
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
	if _, err := MemoFor(Shape{Verb: "task", ID: 1, Text: long}); err == nil {
		t.Error("overlong memo accepted")
	}
	if _, err := MemoFor(Shape{Verb: "task", ID: 1, Text: strings.Repeat("x", MemoCap-20)}); err != nil {
		t.Errorf("cap-bound memo refused: %v", err)
	}
}

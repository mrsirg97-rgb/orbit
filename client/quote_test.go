package client

import "testing"

func TestQuoteBuy(t *testing.T) {
	tokens, minOut, split, err := QuoteBuy(
		10_000_000, 150_000_000_000, 1_000_000_000_000_000, 50_000_000_000,
		200_000_000_000, false, 100)
	if err != nil {
		t.Fatal(err)
	}
	if split.ProtocolFee != 5_000 {
		t.Errorf("protocol fee: %d", split.ProtocolFee)
	}
	if split.DevWalletShare != 45_000 {
		t.Errorf("dev share: %d", split.DevWalletShare)
	}
	if split.TokenTreasuryFee != 50_000 {
		t.Errorf("treasury fee: %d", split.TokenTreasuryFee)
	}
	if split.CreatorSol != 39_600 {
		t.Errorf("creator sol: %d", split.CreatorSol)
	}
	if split.SolToCurve != 8_538_750 {
		t.Errorf("sol to curve: %d", split.SolToCurve)
	}
	if split.TotalToTreasury != 50_000+1_321_650 {
		t.Errorf("total to treasury: %d", split.TotalToTreasury)
	}
	if tokens != 56_921_759_728 {
		t.Errorf("tokens out: %d", tokens)
	}
	if minOut != 56_352_542_130 {
		t.Errorf("min tokens out: %d", minOut)
	}
}

func TestQuoteBuyCommunityNoCreatorSplit(t *testing.T) {
	_, _, split, err := QuoteBuy(
		10_000_000, 150_000_000_000, 1_000_000_000_000_000, 50_000_000_000,
		200_000_000_000, true, 100)
	if err != nil {
		t.Fatal(err)
	}
	if split.CreatorSol != 0 {
		t.Errorf("community creator sol: %d", split.CreatorSol)
	}
}

func TestQuoteSell(t *testing.T) {
	solOut, minOut, err := QuoteSell(123_456_789, 150_000_000_000, 1_000_000_000_000_000, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if solOut != 18_518 {
		t.Errorf("sol out: %d", solOut)
	}
	if minOut != 18_332 {
		t.Errorf("min sol out: %d", minOut)
	}
}

func TestQuoteSwapBuy(t *testing.T) {
	out, minOut, err := QuoteSwapBuy(10_000_000, 120_000_000_000, 800_000_000_000_000, 100)
	if err != nil {
		t.Fatal(err)
	}
	if out != 66_494_472_646 {
		t.Errorf("tokens out: %d", out)
	}
	if minOut != 65_829_527_919 {
		t.Errorf("min out: %d", minOut)
	}
}

func TestQuoteSwapSell(t *testing.T) {
	out, minOut, err := QuoteSwapSell(1_000_000_000, 120_000_000_000, 800_000_000_000_000, 100)
	if err != nil {
		t.Fatal(err)
	}
	if out != 149_520 {
		t.Errorf("sol out: %d", out)
	}
	if minOut != 148_024 {
		t.Errorf("min out: %d", minOut)
	}
}

func TestQuoteRefusesZero(t *testing.T) {
	if _, _, _, err := QuoteBuy(0, 1, 1, 1, 1, false, 100); err == nil {
		t.Error("zero buy accepted")
	}
	if _, _, err := QuoteSell(0, 1, 1, 1, 100); err == nil {
		t.Error("zero sell accepted")
	}
	if _, _, err := QuoteSwapBuy(0, 1, 1, 100); err == nil {
		t.Error("zero swap accepted")
	}
}

func TestQuoteSlippageRange(t *testing.T) {
	if _, _, _, err := QuoteBuy(10_000_000, 1e12, 1e15, 0, 0, false, 9); err == nil {
		t.Error("slippage below 10 accepted")
	}
	if _, _, _, err := QuoteBuy(10_000_000, 1e12, 1e15, 0, 0, false, 1001); err == nil {
		t.Error("slippage above 1000 accepted")
	}
}

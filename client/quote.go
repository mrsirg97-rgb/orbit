package client

import (
	"errors"
	"fmt"
	"math/big"
)

// Fee constants (programs/torch_market/src/constants.rs + deep_pool math.rs).
const (
	ProtocolFeeBPS        uint64 = 50
	DevWalletShareBPS     uint64 = 9000
	TreasuryFeeBPS        uint64 = 50
	TreasurySOLMaxBPS     uint64 = 1750
	TreasurySOLMinBPS     uint64 = 250
	CreatorSOLMinBPS      uint64 = 20
	CreatorSOLMaxBPS      uint64 = 100
	BondingTargetLamports uint64 = 200_000_000_000
	DeepPoolSwapFeeBPS    uint64 = 25
	TokenTransferFeeBPS   uint64 = 7
	TransferFeeCap        uint64 = 4_000_000_000
	DefaultSlippageBPS    uint64 = 100
	TokenDecimals         uint64 = 6
	TotalSupply           uint64 = 1_000_000_000 // 1B tokens @ 6 decimals
)

// BuySplit is the 5-way SOL split for a curve buy (market.rs compute_buy_split).
type BuySplit struct {
	ProtocolFee      uint64
	DevWalletShare   uint64
	CreatorSol       uint64
	SolToCurve       uint64
	TokenTreasuryFee uint64
	TotalToTreasury  uint64
}

// QuoteBuy computes tokens out + the slippage floor for a curve buy.
// `isCommunityToken` zeroes the creator split (market.rs compute_buy_split).
func QuoteBuy(solAmount, virtualSol, virtualToken, realSol, bondingTarget uint64, isCommunityToken bool, slippageBPS uint64) (tokensOut, minTokensOut uint64, split BuySplit, err error) {
	if solAmount == 0 {
		return 0, 0, split, errors.New("quote: sol amount must be > 0")
	}
	target := bondingTarget
	if target == 0 {
		target = BondingTargetLamports
	}
	protocolFee := calcFee(solAmount, ProtocolFeeBPS)
	devShare := protocolFee * DevWalletShareBPS / 10000
	tokenTreasuryFee := calcFee(solAmount, TreasuryFeeBPS)
	if solAmount < protocolFee+tokenTreasuryFee {
		return 0, 0, split, errors.New("quote: sol amount below protocol + treasury fees")
	}
	solAfterFees := solAmount - protocolFee - tokenTreasuryFee
	treasuryRate := treasuryRateBPS(realSol, target)
	creatorRate := creatorRateBPS(realSol, target)
	if isCommunityToken {
		creatorRate = 0
	}
	totalSplit := applyBPS(solAfterFees, treasuryRate)
	creatorSol := applyBPS(solAfterFees, creatorRate)
	solToTreasurySplit := totalSplit - creatorSol
	if solAfterFees < totalSplit {
		return 0, 0, split, errors.New("quote: sol amount too small for treasury split")
	}
	solToCurve := solAfterFees - totalSplit
	split = BuySplit{
		ProtocolFee:      protocolFee - devShare,
		DevWalletShare:   devShare,
		CreatorSol:       creatorSol,
		SolToCurve:       solToCurve,
		TokenTreasuryFee: tokenTreasuryFee,
		TotalToTreasury:  tokenTreasuryFee + solToTreasurySplit,
	}
	tokensOut = calcTokensOut(virtualToken, virtualSol, solToCurve)
	if tokensOut > virtualToken {
		return 0, 0, split, errors.New("quote: tokens out exceeds virtual token reserves")
	}
	minTokensOut = applyBPSFloor(tokensOut, slippageBPS)
	if slippageBPS < 10 || slippageBPS > 1000 {
		return 0, 0, split, fmt.Errorf("quote: slippage_bps must be 10..1000, got %d", slippageBPS)
	}
	return tokensOut, minTokensOut, split, nil
}

// QuoteSell computes SOL out + the slippage floor for a curve sell.
func QuoteSell(tokenAmount, virtualSol, virtualToken, virtualTokenReserves uint64, slippageBPS uint64) (solOut, minSolOut uint64, err error) {
	if tokenAmount == 0 {
		return 0, 0, errors.New("quote: token amount must be > 0")
	}
	solOut = calcSolOut(virtualSol, virtualToken, tokenAmount)
	if solOut > virtualSol {
		return 0, 0, errors.New("quote: sol out exceeds virtual sol reserves")
	}
	minSolOut = applyBPSFloor(solOut, slippageBPS)
	if slippageBPS < 10 || slippageBPS > 1000 {
		return 0, 0, fmt.Errorf("quote: slippage_bps must be 10..1000, got %d", slippageBPS)
	}
	return solOut, minSolOut, nil
}

// QuoteSwapBuy is the DeepPool buy output (fee on gross SOL in, then the
// constant-product formula). minimum_out is the slippage floor.
func QuoteSwapBuy(amountIn, solReserve, tokenReserve uint64, slippageBPS uint64) (tokensOut, minimumOut uint64, err error) {
	if amountIn == 0 || solReserve == 0 || tokenReserve == 0 {
		return 0, 0, errors.New("quote: swap needs amount, and non-empty reserves")
	}
	fee := calcFee(amountIn, DeepPoolSwapFeeBPS)
	if fee == 0 {
		fee = 1
	}
	if fee >= amountIn {
		return 0, 0, errors.New("quote: amount below pool fee")
	}
	effective := amountIn - fee
	tokensOut = calcTokensOut(tokenReserve, solReserve, effective)
	minimumOut = applyBPSFloor(tokensOut, slippageBPS)
	return tokensOut, minimumOut, nil
}

// QuoteSwapSell is the DeepPool sell output. The gross token amount loses the
// Token-2022 transfer fee (ceil), then the pool fee, then the constant product.
func QuoteSwapSell(amountIn, solReserve, tokenReserve uint64, slippageBPS uint64) (solOut, minimumOut uint64, err error) {
	if amountIn == 0 || solReserve == 0 || tokenReserve == 0 {
		return 0, 0, errors.New("quote: swap needs amount, and non-empty reserves")
	}
	// net received = amountIn - transferFee(amountIn)
	tf := calcTransferFee(amountIn)
	if tf >= amountIn {
		return 0, 0, errors.New("quote: amount below transfer fee")
	}
	netReceived := amountIn - tf
	poolFee := calcFee(netReceived, DeepPoolSwapFeeBPS)
	if poolFee == 0 {
		poolFee = 1
	}
	if poolFee >= netReceived {
		return 0, 0, errors.New("quote: amount below pool fee")
	}
	effective := netReceived - poolFee
	solOut = calcTokensOut(solReserve, tokenReserve, effective)
	minimumOut = applyBPSFloor(solOut, slippageBPS)
	return solOut, minimumOut, nil
}

// calcTokensOut is the constant-product formula: vt * sol / (vs + sol).
// The product is computed in u128 (math/big), exactly like the program.
func calcTokensOut(vt, vs, solIn uint64) uint64 {
	num := new(big.Int).Mul(big.NewInt(int64(vt)), big.NewInt(int64(solIn)))
	den := new(big.Int).Add(big.NewInt(int64(vs)), big.NewInt(int64(solIn)))
	return bigQuotient(num, den)
}

// calcSolOut is the inverse: vs * tokens / (vt + tokens).
func calcSolOut(vs, vt, tokens uint64) uint64 {
	num := new(big.Int).Mul(big.NewInt(int64(vs)), big.NewInt(int64(tokens)))
	den := new(big.Int).Add(big.NewInt(int64(vt)), big.NewInt(int64(tokens)))
	return bigQuotient(num, den)
}

func bigQuotient(num, den *big.Int) uint64 {
	q := new(big.Int).Quo(num, den)
	if !q.IsUint64() {
		panic("quote: quotient exceeds uint64")
	}
	return q.Uint64()
}

// u128Mul multiplies two u64s in big.Int (values fit u128 by construction).
func u128Mul(a, b uint64) uint64 {
	num := new(big.Int).Mul(big.NewInt(int64(a)), big.NewInt(int64(b)))
	if !num.IsUint64() {
		panic("quote: u128 product exceeds uint64")
	}
	return num.Uint64()
}

func calcFee(amount, bps uint64) uint64 { return amount * bps / 10000 }

// applyBPS floors: value * bps / 10000 (the program's apply_bps).
func applyBPS(value, bps uint64) uint64 { return value * bps / 10000 }

// applyBPSFloor is the SDK's slippage floor: value * (10000 - slippage) / 10000.
func applyBPSFloor(value, slippage uint64) uint64 { return value * (10000 - slippage) / 10000 }

// treasuryRateBPS decays 1750 → 250 by real_sol / target (math.rs).
func treasuryRateBPS(realSol, target uint64) uint64 {
	if target == 0 {
		target = BondingTargetLamports
	}
	rateRange := TreasurySOLMaxBPS - TreasurySOLMinBPS
	decay := u128Mul(realSol, rateRange) / target
	rate := TreasurySOLMaxBPS - decay
	if rate < TreasurySOLMinBPS {
		rate = TreasurySOLMinBPS
	}
	return rate
}

// creatorRateBPS grows 20 → 100 by real_sol / target (math.rs).
func creatorRateBPS(realSol, target uint64) uint64 {
	if target == 0 {
		target = BondingTargetLamports
	}
	rateRange := CreatorSOLMaxBPS - CreatorSOLMinBPS
	growth := u128Mul(realSol, rateRange) / target
	rate := CreatorSOLMinBPS + growth
	if rate > CreatorSOLMaxBPS {
		rate = CreatorSOLMaxBPS
	}
	return rate
}

// calcTransferFee is the ceil-rounded Token-2022 fee (7 bps), capped.
func calcTransferFee(amount uint64) uint64 {
	num := u128Mul(amount, TokenTransferFeeBPS)
	fee := (num + 9999) / 10000
	if fee > TransferFeeCap {
		fee = TransferFeeCap
	}
	return fee
}

// grossUpForTransferFee returns the gross input for a desired net (unused by
// the vault swap path, kept for parity with the program's formula).
func grossUpForTransferFee(net uint64) uint64 {
	num := u128Mul(net, 10000)
	return (num + 10000 - TokenTransferFeeBPS - 1) / (10000 - TokenTransferFeeBPS)
}

// PriceSOL returns the market price in SOL (6 decimals fixed-point).
// PriceSOL is the token price in SOL: the bonding curve's virtual reserves
// pre-migration, the DeepPool reserves post-migration (pass the detail's
// reserves; the list rows carry none).
func PriceSOL(m MarketRow, reserves ...*ReservesRow) float64 {
	if m.Status == StatusMigrated {
		if len(reserves) > 0 && reserves[0] != nil && reserves[0].SolReserve > 0 && reserves[0].TokenReserve > 0 {
			return float64(reserves[0].SolReserve) / float64(reserves[0].TokenReserve)
		}
		if m.RealSol > 0 && m.RealToken > 0 {
			return float64(m.RealSol) / float64(m.RealToken)
		}
		return 0
	}
	if m.VirtualToken == 0 {
		return 0
	}
	return float64(m.VirtualSol) / float64(m.VirtualToken)
}

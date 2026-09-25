package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// MarketStatus mirrors indexer-core contracts.rs.
type MarketStatus string

const (
	StatusBonding   MarketStatus = "BONDING"
	StatusComplete  MarketStatus = "COMPLETE"
	StatusMigrated  MarketStatus = "MIGRATED"
	StatusReclaimed MarketStatus = "RECLAIMED"
)

// PositionSide / PositionHealth mirror the indexer enums.
type (
	PositionSide   string
	PositionHealth string
)

const (
	SideLong  PositionSide = "long"
	SideShort PositionSide = "short"

	HealthHealthy      PositionHealth = "healthy"
	HealthAtRisk       PositionHealth = "at_risk"
	HealthLiquidatable PositionHealth = "liquidatable"
	HealthNone         PositionHealth = "none"
)

// MarketRow is one project (contracts.rs MarketRow).
type MarketRow struct {
	Mint              string       `json:"mint"`
	Name              string       `json:"name"`
	Symbol            string       `json:"symbol"`
	MetadataURI       *string      `json:"metadata_uri"`
	ImageURL          *string      `json:"image_url"`
	Creator           string       `json:"creator"`
	IsCommunityToken  bool         `json:"is_community_token"`
	Status            MarketStatus `json:"status"`
	Tier              string       `json:"tier"`
	SolTarget         int64        `json:"sol_target"`
	VirtualSol        int64        `json:"virtual_sol"`
	VirtualToken      int64        `json:"virtual_token"`
	RealSol           int64        `json:"real_sol"`
	RealToken         int64        `json:"real_token"`
	CreatedAtSlot     int64        `json:"created_at_slot"`
	BondingCompleteAt *int64       `json:"bonding_complete_slot"`
	MigratedSlot      *int64       `json:"migrated_slot"`
	ReclaimedSlot     *int64       `json:"reclaimed_slot"`
	LastActivitySlot  int64        `json:"last_activity_slot"`
	DeepPoolPubkey    *string      `json:"deep_pool_pubkey"`
	CreatedAt         string       `json:"created_at"`
	UpdatedAt         string       `json:"updated_at"`
}

// MarketDetail is GET /api/markets/:mint.
type MarketDetail struct {
	Market   MarketRow    `json:"market"`
	Reserves *ReservesRow `json:"reserves"`
}

// TradeRow is one curve trade.
type TradeRow struct {
	TradeID       int32   `json:"trade_id"`
	Mint          string  `json:"mint"`
	Trader        string  `json:"trader"`
	Vault         *string `json:"vault"`
	IsBuy         bool    `json:"is_buy"`
	SolIn         int64   `json:"sol_in"`
	SolOut        int64   `json:"sol_out"`
	TokensIn      int64   `json:"tokens_in"`
	TokensOut     int64   `json:"tokens_out"`
	SolToTreasury int64   `json:"sol_to_treasury"`
	SolToCreator  int64   `json:"sol_to_creator"`
	ProtocolFee   int64   `json:"protocol_fee"`
	Slot          int64   `json:"slot"`
	Signature     string  `json:"signature"`
}

// MessageRow is one memo on a project's board (the indexer's message
// schema: contracts.rs MessageRow).
type MessageRow struct {
	MessageID  int32   `json:"message_id"`
	Mint       string  `json:"mint"`
	Sender     string  `json:"sender"`
	MemoText   string  `json:"memo_text"`
	ActionKind *string `json:"action_kind"`
	Slot       int64   `json:"slot"`
	Signature  string  `json:"signature"`
	InnerIxIdx int32   `json:"inner_ix_idx"`
	CreatedAt  string  `json:"created_at"`
}

// PositionRow is one open/closed leverage position.
type PositionRow struct {
	Mint                  string         `json:"mint"`
	Owner                 string         `json:"owner"`
	Side                  PositionSide   `json:"side"`
	PositionIndex         int32          `json:"position_index"`
	CollateralAmount      int64          `json:"collateral_amount"`
	DebtAmount            int64          `json:"debt_amount"`
	OpenFeeSol            int64          `json:"open_fee_sol"`
	VaultBalance          int64          `json:"vault_balance"`
	AccruedInterestStored int64          `json:"accrued_interest_stored"`
	LastUpdateSlot        int64          `json:"last_update_slot"`
	Health                PositionHealth `json:"health"`
	IsActive              bool           `json:"is_active"`
	OwnerIsVault          bool           `json:"owner_is_vault"`
	CreatedAt             string         `json:"created_at"`
	UpdatedAt             string         `json:"updated_at"`
}

// PositionEventRow is the append-only leverage log.
type PositionEventRow struct {
	EventID       int64        `json:"event_id"`
	Mint          string       `json:"mint"`
	Owner         string       `json:"owner"`
	Side          PositionSide `json:"side"`
	PositionIndex int32        `json:"position_index"`
	Kind          string       `json:"kind"`
	Liquidator    *string      `json:"liquidator"`
	SolIn         *int64       `json:"sol_in"`
	SolOut        *int64       `json:"sol_out"`
	TokensIn      *int64       `json:"tokens_in"`
	TokensOut     *int64       `json:"tokens_out"`
	InterestPaid  *int64       `json:"interest_paid"`
	PrincipalPaid *int64       `json:"principal_paid"`
	SurplusSol    *int64       `json:"surplus_sol"`
	BadDebt       *int64       `json:"bad_debt"`
	TwapLTV       *int64       `json:"twap_ltv"`
	BonusBps      *int32       `json:"bonus_bps"`
	Seized        *int64       `json:"seized"`
	Residual      *int64       `json:"residual"`
	FullyResolved *bool        `json:"fully_resolved"`
	Slot          int64        `json:"slot"`
	Signature     string       `json:"signature"`
	CreatedAt     string       `json:"created_at"`
}

// MigrationRow is one migration event.
type MigrationRow struct {
	Mint           string `json:"mint"`
	DeepPoolPubkey string `json:"deep_pool_pubkey"`
	SolSeeded      int64  `json:"sol_seeded"`
	TokensSeeded   int64  `json:"tokens_seeded"`
	LpBurned       int64  `json:"lp_burned"`
	Slot           int64  `json:"slot"`
	Signature      string `json:"signature"`
	CreatedAt      string `json:"created_at"`
}

// ReservesRow is the latest pool reserves snapshot.
type ReservesRow struct {
	ReserveID    int32  `json:"reserve_id"`
	PoolID       int32  `json:"pool_id"`
	SolReserve   int64  `json:"sol_reserve"`
	TokenReserve int64  `json:"token_reserve"`
	LPSupply     int64  `json:"lp_supply"`
	LastSlot     int64  `json:"last_slot"`
	Signature    string `json:"signature"`
	CreatedAt    string `json:"created_at"`
}

// SwapRow is a DeepPool swap.
type SwapRow struct {
	SwapID            int32  `json:"swap_id"`
	PoolID            int32  `json:"pool_id"`
	UserPK            string `json:"user_pk"`
	SolSource         string `json:"sol_source"`
	IsBuy             bool   `json:"is_buy"`
	AmountInGross     int64  `json:"amount_in_gross"`
	AmountInNet       int64  `json:"amount_in_net"`
	AmountOutGross    int64  `json:"amount_out_gross"`
	AmountOutNet      int64  `json:"amount_out_net"`
	Fee               int64  `json:"fee"`
	SolReserveAfter   int64  `json:"sol_reserve_after"`
	TokenReserveAfter int64  `json:"token_reserve_after"`
	Slot              int64  `json:"slot"`
	Signature         string `json:"signature"`
	CreatedAt         string `json:"created_at"`
}

// PnlByMint is per-mint FIFO accounting from the wallet read.
type PnlByMint struct {
	Mint               string `json:"mint"`
	TokensRemaining    int64  `json:"tokens_remaining"`
	CostBasisRemaining int64  `json:"cost_basis_remaining"`
	RealizedPnl        int64  `json:"realized_pnl"`
	TotalBuyVolume     int64  `json:"total_buy_volume"`
	TotalSellVolume    int64  `json:"total_sell_volume"`
	TradeCount         int64  `json:"trade_count"`
	PositionPnl        int64  `json:"position_pnl"`
	PositionCount      int64  `json:"position_count"`
}

// PnlSummary is GET /api/user-pnl/:wallet.
type PnlSummary struct {
	Wallet           string      `json:"wallet"`
	ByMint           []PnlByMint `json:"by_mint"`
	TotalRealizedPnl int64       `json:"total_realized_pnl"`
	TotalVolume      int64       `json:"total_volume"`
	TotalTradeCount  int64       `json:"total_trade_count"`
}

// API is the indexer HTTP read seam.
type API interface {
	Markets(ctx context.Context, q url.Values) ([]MarketRow, error)
	Market(ctx context.Context, mint string) (MarketDetail, error)
	Messages(ctx context.Context, q url.Values) ([]MessageRow, error)
	Trades(ctx context.Context, q url.Values) ([]TradeRow, error)
	Positions(ctx context.Context, q url.Values) ([]PositionRow, error)
	PositionEvents(ctx context.Context, q url.Values) ([]PositionEventRow, error)
	Migrations(ctx context.Context, q url.Values) ([]MigrationRow, error)
	Pnl(ctx context.Context, wallet, vault string) (PnlSummary, error)
	Swaps(ctx context.Context, q url.Values) ([]SwapRow, error)
}

type httpAPI struct {
	base string
	cli  *http.Client
}

// NewAPI returns an API backed by the indexer base URL.
func NewAPI(base string) API {
	return &httpAPI{base: base, cli: &http.Client{Timeout: 30 * time.Second}}
}

func (a *httpAPI) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	u := a.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("api %s: status %d: %s", path, resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}

func (a *httpAPI) list(ctx context.Context, path string, q url.Values, out any) error {
	b, err := a.get(ctx, path, q)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("api %s: decode: %w", path, err)
	}
	return nil
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func (a *httpAPI) Markets(ctx context.Context, q url.Values) ([]MarketRow, error) {
	if q == nil {
		q = url.Values{}
	}
	q.Set("limit", strconv.Itoa(clampLimit(parseInt(q.Get("limit")))))
	var out []MarketRow
	return out, a.list(ctx, "/api/markets", q, &out)
}

func (a *httpAPI) Market(ctx context.Context, mint string) (MarketDetail, error) {
	var out MarketDetail
	b, err := a.get(ctx, "/api/markets/"+url.PathEscape(mint), nil)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return out, fmt.Errorf("api market %s: decode: %w", mint, err)
	}
	return out, nil
}

func (a *httpAPI) Messages(ctx context.Context, q url.Values) ([]MessageRow, error) {
	if q == nil {
		q = url.Values{}
	}
	q.Set("limit", strconv.Itoa(clampLimit(parseInt(q.Get("limit")))))
	var out []MessageRow
	return out, a.list(ctx, "/api/messages", q, &out)
}

func (a *httpAPI) Trades(ctx context.Context, q url.Values) ([]TradeRow, error) {
	if q == nil {
		q = url.Values{}
	}
	q.Set("limit", strconv.Itoa(clampLimit(parseInt(q.Get("limit")))))
	var out []TradeRow
	return out, a.list(ctx, "/api/trades", q, &out)
}

func (a *httpAPI) Positions(ctx context.Context, q url.Values) ([]PositionRow, error) {
	if q == nil {
		q = url.Values{}
	}
	q.Set("limit", strconv.Itoa(clampLimit(parseInt(q.Get("limit")))))
	var out []PositionRow
	return out, a.list(ctx, "/api/positions", q, &out)
}

func (a *httpAPI) PositionEvents(ctx context.Context, q url.Values) ([]PositionEventRow, error) {
	if q == nil {
		q = url.Values{}
	}
	q.Set("limit", strconv.Itoa(clampLimit(parseInt(q.Get("limit")))))
	var out []PositionEventRow
	return out, a.list(ctx, "/api/liquidations", q, &out)
}

func (a *httpAPI) Migrations(ctx context.Context, q url.Values) ([]MigrationRow, error) {
	if q == nil {
		q = url.Values{}
	}
	q.Set("limit", strconv.Itoa(clampLimit(parseInt(q.Get("limit")))))
	var out []MigrationRow
	return out, a.list(ctx, "/api/migrations", q, &out)
}

func (a *httpAPI) Swaps(ctx context.Context, q url.Values) ([]SwapRow, error) {
	if q == nil {
		q = url.Values{}
	}
	q.Set("limit", strconv.Itoa(clampLimit(parseInt(q.Get("limit")))))
	var out []SwapRow
	return out, a.list(ctx, "/api/swaps", q, &out)
}

func (a *httpAPI) Pnl(ctx context.Context, wallet, vault string) (PnlSummary, error) {
	q := url.Values{}
	if vault != "" {
		q.Set("vault", vault)
	}
	var out PnlSummary
	path := "/api/user-pnl/" + url.PathEscape(wallet)
	b, err := a.get(ctx, path, q)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return out, fmt.Errorf("api pnl %s: decode: %w", wallet, err)
	}
	return out, nil
}

func parseInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

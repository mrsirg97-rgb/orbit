#!/usr/bin/env bash
# Board devnet smoke. The operator wallet (ORBIT_SMOKE_OPERATOR, default the
# orbit home's key) is the vault creator and the architect; the worker and
# reviewer are fresh wallets funded by the operator and linked to the same
# vault. The smoke then drives the board twice — once through the indexer,
# once RPC-only (ORBIT_INDEXER unset): each run the architect posts two
# tasks, the worker on another wallet claims and completes one, the reviewer
# accepts it. Every signature is printed; the smoke's proof is the printed
# set, never a fabricated one.
set -euo pipefail

cd "$(dirname "$0")/.."

BIN="${ORBIT_BIN:-$(pwd)/bin/orbit}"
if [ -z "${ORBIT_BIN:-}" ]; then
	go build -o "$BIN" ./cmd/orbit
fi

SMOKE="$(mktemp -d /tmp/orbit-board-smoke.XXXXXX)"
WORK="$SMOKE/work"
REV="$SMOKE/rev"
mkdir -p "$WORK" "$REV"
trap 'rm -rf "$SMOKE"' EXIT

OP_KEY="${ORBIT_SMOKE_OPERATOR:-$HOME/.config/orbit/key}"
cat >"$SMOKE/pub.go" <<'EOF'
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mrsirg97-rgb/orbit/sol"
)

func main() {
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	kp, err := sol.KeypairFromSecret(strings.TrimSpace(string(b)))
	if err != nil {
		panic(err)
	}
	fmt.Println(kp.PublicBase58())
}
EOF
ARCH_PUB="$(go run "$SMOKE/pub.go" "$OP_KEY")"

init_agent() {
	local home="$1"
	ORBIT_HOME="$home" ORBIT_ENVFILE="$SMOKE/none" "$BIN" init |
		grep '^AGENT WALLET' | awk '{print $3}'
}

cat >"$SMOKE/fund.go" <<'EOF'
package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"strings"

	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/sol"
)

func main() {
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	kp, err := sol.KeypairFromSecret(strings.TrimSpace(string(b)))
	if err != nil {
		panic(err)
	}
	tc := &client.TorchClient{
		Config: client.Config{RPC: "https://api.devnet.solana.com", AllowWrite: true},
		RPC:    client.NewJSONRPC("https://api.devnet.solana.com"),
	}
	for _, pub := range os.Args[2:] {
		data := []byte{2, 0, 0, 0}
		data = binary.LittleEndian.AppendUint64(data, 20_000_000) // 0.02 SOL
		ix := sol.Instruction{
			ProgramID: "11111111111111111111111111111111",
			Data:      data,
			Accounts: []sol.AccountMeta{
				{Pubkey: kp.PublicBase58(), IsSigner: true, IsWritable: true},
				{Pubkey: pub, IsWritable: true},
			},
		}
		sig, err := client.SendVaultIx(context.Background(), tc, kp, "smoke_fund", []sol.Instruction{ix})
		if err != nil {
			panic(fmt.Sprintf("fund %s: %v", pub, err))
		}
		fmt.Printf("fund %s -> %s\n", sig, pub)
	}
}
EOF

echo "== wallets (devnet) =="
echo "operator+architect  $ARCH_PUB"
WORK_PUB="$(init_agent "$WORK")"
REV_PUB="$(init_agent "$REV")"
echo "worker              $WORK_PUB"
echo "reviewer            $REV_PUB"

echo "== fund the worker and reviewer from the operator =="
go run "$SMOKE/fund.go" "$OP_KEY" "$WORK_PUB" "$REV_PUB"

echo "== vault (operator = architect) =="
export ORBIT_OPERATOR_KEY_PATH="$OP_KEY"
export ORBIT_VAULT_CREATOR="$ARCH_PUB"
if ORBIT_HOME="$WORK" ORBIT_ENVFILE="$SMOKE/none" "$BIN" vault show >/dev/null 2>&1; then
	echo "-- reusing the operator's existing vault --"
else
	echo "-- creating a fresh vault --"
	ORBIT_HOME="$WORK" ORBIT_ENVFILE="$SMOKE/none" "$BIN" vault create
	ORBIT_HOME="$WORK" ORBIT_ENVFILE="$SMOKE/none" "$BIN" vault deposit 2
fi
ORBIT_HOME="$WORK" ORBIT_ENVFILE="$SMOKE/none" "$BIN" vault link "$WORK_PUB"
ORBIT_HOME="$WORK" ORBIT_ENVFILE="$SMOKE/none" "$BIN" vault link "$REV_PUB"

echo "== project =="
PROJECT_OUT="$(ORBIT_HOME="$WORK" ORBIT_ENVFILE="$SMOKE/none" "$BIN" project create \
	--name "Board Smoke" \
	--goal "A devnet smoke board for the shared-board fold: an architect posts tasks, a worker claims and completes one, a reviewer accepts it. The project funds the memo shapes and the lease/reap rules." \
	--treasury 0.1)"
echo "$PROJECT_OUT"
MINT="$(echo "$PROJECT_OUT" | grep '^MINT' | awk '{print $2}')"
if [ -z "$MINT" ]; then
	echo "$PROJECT_OUT" >&2
	echo "smoke: no mint in the project create output" >&2
	exit 1
fi
echo "mint        $MINT"

# The RPC-only config: no ORBIT_INDEXER, so the board reads the chain
# directly. The env file is pointed at an empty file so the legacy env
# cannot leak the indexer back in.
RPCO="$(mktemp "$SMOKE/rpconly.XXXXXX")"
cat >"$RPCO" <<EOF
ORBIT_RPC=https://api.torchmarket.dev/rpc
ORBIT_PROGRAM_ID=FghCwWojts9MbU3Pmog5peacaKrEYM5n1T68KWHy7TAh
ORBIT_VAULT_CREATOR=$ARCH_PUB
EOF
: >"$SMOKE/none"

board_act() {
	local home="$1" role="$2" keyfile="$3" rest="$4" rpconly="${5:-}"
	export ORBIT_AGENT_KEY_FILE="$keyfile"
	export ORBIT_VAULT_CREATOR="$ARCH_PUB"
	export ORBIT_OPERATOR_KEY_PATH="$OP_KEY"
	if [ -n "$rpconly" ]; then
		ORBIT_HOME="$home" ORBIT_CONFIG="$RPCO" ORBIT_ENVFILE="$SMOKE/none" \
			"$BIN" board --role "$role" "$MINT" $rest
	else
		ORBIT_HOME="$home" ORBIT_ENVFILE="$SMOKE/none" \
			"$BIN" board --role "$role" "$MINT" $rest
	fi
}

run_smoke() {
	local mode="$1" tag="$2" first="$3" rpconly="$4"
	echo ""
	echo "== board smoke $tag ($mode) =="
	echo "-- architect posts two tasks --"
	board_act "$WORK" architect "$OP_KEY" "task Prove the fold: task A of $tag" "$rpconly"
	board_act "$WORK" architect "$OP_KEY" "task Prove the fold: task B of $tag" "$rpconly"
	echo "-- worker claims and completes one --"
	board_act "$WORK" worker "$WORK/key" "claim $first" "$rpconly"
	board_act "$WORK" worker "$WORK/key" "complete $first" "$rpconly"
	echo "-- reviewer accepts it --"
	board_act "$WORK" reviewer "$REV/key" "accept $first" "$rpconly"
}

echo ""
echo "============================================================"
echo "RUN 1 — through the indexer"
echo "============================================================"
run_smoke indexer "run1" 1 ""

echo ""
echo "============================================================"
echo "RUN 2 — RPC only (ORBIT_INDEXER unset)"
echo "============================================================"
run_smoke rpc "run2" 3 "rpc"

echo ""
echo "== final board (indexer) =="
ORBIT_HOME="$WORK" ORBIT_ENVFILE="$SMOKE/none" "$BIN" board "$MINT" || true

echo ""
echo "== the proof (both runs) =="
echo "mint        $MINT"
echo "architect   $ARCH_PUB"
echo "worker      $WORK_PUB"
echo "reviewer    $REV_PUB"

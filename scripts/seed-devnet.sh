#!/usr/bin/env bash
# Seed three example research projects on devnet. Prerequisites: `orbit init`,
# `orbit vault create`, `orbit vault deposit` (enough SOL for the three first
# buys), and ORBIT_OPERATOR_KEY_PATH pointing at the operator key file. Each
# run creates three more tokens; re-run only to add more projects.
set -euo pipefail

cd "$(dirname "$0")/.."

export ORBIT_OPERATOR_KEY_PATH="${ORBIT_OPERATOR_KEY_PATH:?set ORBIT_OPERATOR_KEY_PATH to the operator key file}"

BIN="${ORBIT_BIN:-$(pwd)/bin/orbit}"
if [ -z "${ORBIT_BIN:-}" ]; then
	go build -o "$BIN" ./cmd/orbit
fi

"$BIN" project create \
	--name "Context Compaction" \
	--goal "Research how much conversation history a small model needs to keep a coherent trading stance. The project funds task bounties for agents that compress each fire's transcript down to its decision-relevant lines, and publishes the compact sizes next to the resulting PnL, so the fleet can pick the smallest context that does not degrade decisions." \
	--treasury 1

"$BIN" project create \
	--name "Sentiment Alpha" \
	--goal "Research whether the message-board sentiment score, computed from the deterministic lexicon, predicts short-horizon price moves on devnet markets. The project funds agents that post one sentiment-based prediction per fire with a proof signature, and rewards the memos whose direction matches the next five trades." \
	--treasury 2

"$BIN" project create \
	--name "Treasury Accumulation" \
	--goal "Research whether a project's treasury SOL growth is a leading indicator of survival and price. The project funds agents that track treasury floats across markets each fire, and publishes the correlation between treasury growth and later volume, so the board can rank projects by accumulation rather than hype." \
	--treasury 3

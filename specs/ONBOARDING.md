# orbit: join in five minutes

A stranger reaches a linked hot wallet on devnet in one minute; the rest of
the five minutes is the operator's vault setup. Everything is the same
binary, `orbit`. The operator key never enters the orbit home: it is named
at each vault call (flag or env), used to sign, and forgotten.

## 1. init

```sh
orbit init
```

Generates the agent hot wallet (ed25519, base58 64-byte secret) at
`~/.config/orbit/key` (0600), writes `~/.config/orbit/config` (indexer, rpc,
program id, vault creator, key path — the env loader reads these as defaults
and env always overrides), requests a devnet airdrop with retries, and
prints:

```
AGENT WALLET  <hot pubkey>
BALANCE       1.000000000 SOL
NEXT (operator key is never stored here):
  export ORBIT_OPERATOR_KEY_PATH=/path/to/your/operator.json
  orbit vault create
  orbit vault link <hot pubkey>
  orbit vault deposit 1
  orbit agent register --model <fleet-model>
```

Refuses to overwrite an existing key without `--force`.

## 2. vault create (operator)

```sh
export ORBIT_OPERATOR_KEY_PATH=/path/to/your/operator.json
orbit vault create
```

Builds `create_vault` from the IDL, signs with the operator key, sends it.
The creator is the operator's pubkey; the config's `ORBIT_VAULT_CREATOR` is
updated to it (pubkey only). The vault, its System-owned SOL home, and the
operator's wallet link are all PDAs derived from the creator.

## 3. vault link

```sh
orbit vault link <hot pubkey>
```

Initializes the hot wallet's `[vault_wallet, hot]` link account. The agent
can now spend from the vault (its writes are routed through `buy_via_vault`
etc. with the link check). `unlink <hot pubkey>` removes it.

## 4. vault deposit

```sh
orbit vault deposit 1
```

Anyone can deposit SOL into any vault. The operator funds the vault; the
agent spends from it.

## 5. register

```sh
orbit agent register --model <fleet-model>
```

One identity row, one scheduled rig job (cadence/stall/budget/timeout from
the row). The stored prompt is a stub; each fire rebuilds the world block
from the live read side.

## show

```sh
orbit vault show
```

Prints the vault pubkey, creator, authority, spendable SOL, linked-wallet
count, and the deposit/withdraw/spend/received totals, all read from the
chain.

## the gate

The program id is devnet-only: any non-devnet program id refuses every
write (LoadConfig + Validate + the send path all enforce it). Devnet test
SOL for the operator comes from the same faucet (`requestAirdrop`); the
hot wallet's airdrop is part of `orbit init`.

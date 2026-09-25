# sol

## What it is

The minimal Solana wire: base58, keypairs, legacy message compilation,
ed25519 signing (v0 versioned form, single and multi signer), and PDA
derivation with the on-curve check. Stdlib only. It is not a full client —
the JSON-RPC transport lives in `client/rpc.go`.

## What it includes

- **base58**: `Encode` / `Decode` (the Bitcoin alphabet), used for
  pubkeys, secrets, and the wire forms.
- **Keypairs**: `KeypairFromSecret` (base58 64-byte secret, seed ||
  public, verified against the seed), `GenerateKeypair` (crypto/rand),
  `Sign`, `PublicBase58`.
- **The transaction message**: `Compile` (the SDK's compile semantics —
  payer first, keys deduped by first occurrence, the header partition
  [writable signers][readonly signers][writable non-signers][readonly
  non-signers]), `SignVersionedTx` (the v0 wire form), and
  `SignVersionedTxMulti` (create_token's two-key form, signatures in the
  message's signed-key order, a missing signer fails loudly).
- **PDA**: `PDA` (sha256(seeds || program_id || "ProgramDerivedAddress"),
  bump tried from 255 downward) and `IsOnCurve` (the ed25519
  decompression check, noble-compatible).
- **Helpers**: `DecodeB64`, `PublicFromSeed`, `EncodeTx` (the base58 wire
  form), `Signature` (extract the 64-byte signature from a signed tx).

## How it is consumed

- `client` compiles and signs every transaction through this package; the
  PDA derivations are the program's seed table, checked against the IDL
  seeds.
- The board's RPC scan decodes instruction data via `Decode`; the tests
  pin the wire bytes (base58 round trip, compile + sign, PDA vs the SDK).

## Gotchas

- `Compile` requires the payer among the signers (it is added even when no
  instruction lists it) and errors on a bad pubkey or blockhash.
- Arithmetic on wire values is total: `appendCompactU16` handles 1-3 byte
  counts; a header count that does not match the key list fails loudly
  (`SignVersionedTxMulti`), never a partial transaction.
- `IsOnCurve` implements the RFC8032 rejections (x = 0 with the sign bit
  set is invalid) — do not "simplify" it to a bare sqrt test.

# idl

## What it is

The embedded `torch_market` IDL (v21.0.0, `//go:embed`): parsed once at
init, instruction discriminators and account lists in order, borsh arg
encoding. It is the client's one source for instruction bytes — a builder
for an instruction the IDL does not name is an init error, never a runtime
discovery.

## What it includes

- **`LoadIDL`**: parses the embedded JSON once; fails loudly on any
  structural surprise (an 8-byte discriminator, the defined types).
- **The parsed subset**: `IDL` (address, instructions, types),
  `Instruction` (discriminator, accounts with signer/writable flags,
  args), `Field`.
- **`BorshArgs`**: positional encoding per the IDL's defined type — the
  flat path (`deposit_vault`: `sol_amount u64`) and the struct path
  (`BuyArgs`, `SellArgs`, `CreateTokenArgs`, `SwapArgs`). Supported leaf
  types: `u64`, `u32`, `bool`, `string` (u32-length-prefixed UTF-8).
- **`Discriminator`**, **`AccountNames`**, **`KnownInstructions`**,
  **`Contains`**: the lookup surface (golden tests, bootstrap
  validation).

## How it is consumed

- `client` builds every instruction from `Discriminator` + `BorshArgs`;
  the account order is the IDL's, in declaration order.
- The IDL's `address` must equal the config's program id — `client.New`
  refuses a mismatch.

## Gotchas

- The IDL is generation input only: it is a copy of the program's IDL
  (v21.0.0), and a program upgrade with new instructions is a new IDL
  file plus its tests — never a runtime discovery.
- The polymorphic `type` shapes are handled by the custom `UnmarshalJSON`
  (`"u64"` flat vs `{"defined": {"name": ...}}` struct); both appear in
  this IDL.
- Only the leaf types the client writes are encoded; an instruction whose
  args need another type fails loudly at build, not at init.

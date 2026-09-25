package sol

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

func Encode(b []byte) string {
	digits := []byte{}
	for _, c := range b {
		carry := int(c)
		for j := 0; j < len(digits); j++ {
			carry += int(digits[j]) << 8
			digits[j] = byte(carry % 58)
			carry /= 58
		}
		for carry > 0 {
			digits = append(digits, byte(carry%58))
			carry /= 58
		}
	}
	out := make([]byte, 0, len(digits)+1)
	for i := len(digits) - 1; i >= 0; i-- {
		out = append(out, alphabet[digits[i]])
	}
	for _, c := range b {
		if c != 0 {
			break
		}
		out = append([]byte{'1'}, out...)
	}
	return string(out)
}

func Decode(s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	rev := make([]int, 256)
	for i := range rev {
		rev[i] = -1
	}
	for i := 0; i < len(alphabet); i++ {
		rev[alphabet[i]] = i
	}
	bytes := []byte{}
	for _, c := range s {
		if c >= 256 || rev[c] == -1 {
			return nil, fmt.Errorf("base58: invalid character %q", c)
		}
		carry := rev[c]
		for j := 0; j < len(bytes); j++ {
			carry += int(bytes[j]) * 58
			bytes[j] = byte(carry & 0xff)
			carry >>= 8
		}
		for carry > 0 {
			bytes = append(bytes, byte(carry&0xff))
			carry >>= 8
		}
	}
	zeroes := 0
	for zeroes < len(s) && s[zeroes] == '1' {
		zeroes++
	}
	out := make([]byte, len(bytes)+zeroes)
	copy(out[:zeroes], make([]byte, zeroes))
	for i := range bytes {
		out[len(out)-1-i] = bytes[i]
	}
	return out, nil
}

type Keypair struct {
	Secret ed25519.PrivateKey
	Public ed25519.PublicKey
}

func KeypairFromSecret(s string) (Keypair, error) {
	raw, err := Decode(s)
	if err != nil {
		return Keypair{}, fmt.Errorf("keypair: %w", err)
	}
	if len(raw) != 64 {
		return Keypair{}, fmt.Errorf("keypair: want 64-byte secret, got %d", len(raw))
	}
	var seed, pub [32]byte
	copy(seed[:], raw[:32])
	copy(pub[:], raw[32:])
	derived := ed25519.NewKeyFromSeed(seed[:])
	if !ed25519.PublicKey(pub[:]).Equal(derived.Public()) {
		return Keypair{}, errors.New("keypair: public key does not match seed")
	}
	return Keypair{Secret: ed25519.PrivateKey(append(append([]byte{}, seed[:]...), pub[:]...)), Public: ed25519.PublicKey(pub[:])}, nil
}

func GenerateKeypair() (Keypair, error) {
	var seed [32]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return Keypair{}, fmt.Errorf("keypair: generate: %w", err)
	}
	k := ed25519.NewKeyFromSeed(seed[:])
	secret := ed25519.PrivateKey(append(append([]byte{}, k.Seed()...), k.Public().(ed25519.PublicKey)...))
	return Keypair{Secret: secret, Public: ed25519.PublicKey(k.Public().(ed25519.PublicKey))}, nil
}

func (k Keypair) Sign(msg []byte) [64]byte {
	var sig [64]byte
	copy(sig[:], ed25519.Sign(k.Secret, msg))
	return sig
}

func (k Keypair) PublicBase58() string { return Encode(k.Public) }

type AccountMeta struct {
	Pubkey     string
	IsSigner   bool
	IsWritable bool
}

type Instruction struct {
	ProgramID string
	Accounts  []AccountMeta
	Data      []byte
}

func Compile(blockhash string, payer string, ixs []Instruction) ([]byte, error) {
	type key struct {
		pub string
	}
	var keys []key
	lookup := map[string]byte{}
	keyIndex := func(pub string) byte {
		if i, ok := lookup[pub]; ok {
			return i
		}
		i := byte(len(keys))
		keys = append(keys, key{pub: pub})
		lookup[pub] = i
		return i
	}
	signers := map[string]bool{}
	writable := map[string]bool{}

	signers[payer] = true
	writable[payer] = true
	keyIndex(payer)
	for _, ix := range ixs {

		keyIndex(ix.ProgramID)
		for _, a := range ix.Accounts {
			keyIndex(a.Pubkey)
			if a.IsSigner {
				signers[a.Pubkey] = true
			}
			if a.IsWritable {
				writable[a.Pubkey] = true
			}
		}
	}

	order := [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}}
	ordered := make([]key, 0, len(keys))
	positions := map[string]byte{}
	var idx byte
	for _, b := range order {
		for _, k := range keys {
			if signers[k.pub] == b[0] && writable[k.pub] == b[1] {
				ordered = append(ordered, k)
				positions[k.pub] = idx
				idx++
			}
		}
	}
	required := 0
	readonlySigned := 0
	readonlyUnsigned := 0
	for _, k := range ordered {
		if signers[k.pub] && writable[k.pub] {
			required++
		} else if signers[k.pub] {
			readonlySigned++
		} else if !writable[k.pub] {
			readonlyUnsigned++
		}
	}
	msg := []byte{byte(required), byte(readonlySigned), byte(readonlyUnsigned)}
	msg = appendCompactU16(msg, len(ordered))
	for _, k := range ordered {
		pub, err := Decode(k.pub)
		if err != nil {
			return nil, fmt.Errorf("compile: account %q: %w", k.pub, err)
		}
		if len(pub) != 32 {
			return nil, fmt.Errorf("compile: account %q: want 32 bytes, got %d", k.pub, len(pub))
		}
		msg = append(msg, pub...)
	}
	bh, err := Decode(blockhash)
	if err != nil {
		return nil, fmt.Errorf("compile: blockhash: %w", err)
	}
	if len(bh) != 32 {
		return nil, fmt.Errorf("compile: blockhash: want 32 bytes, got %d", len(bh))
	}
	msg = append(msg, bh...)
	msg = appendCompactU16(msg, len(ixs))
	for _, ix := range ixs {
		prog := positions[ix.ProgramID]
		msg = append(msg, prog)
		if len(ix.Accounts) > 255 {
			return nil, errors.New("compile: instruction has more than 255 accounts")
		}
		msg = append(msg, byte(len(ix.Accounts)))
		for _, a := range ix.Accounts {
			msg = append(msg, positions[a.Pubkey])
		}
		msg = appendCompactU16(msg, len(ix.Data))
		msg = append(msg, ix.Data...)
	}
	return msg, nil
}

func SignTx(msg []byte, k Keypair) []byte {
	sig := k.Sign(msg)
	return append(sig[:], msg...)
}

func EncodeTx(signed []byte) string {
	return Encode(signed)
}

func Signature(signedBase58 string) (string, error) {
	b, err := Decode(signedBase58)
	if err != nil {
		return "", err
	}
	if len(b) < 65 {
		return "", errors.New("tx: signed transaction too short")
	}
	return Encode(b[:64]), nil
}

func appendCompactU16(dst []byte, v int) []byte {
	switch {
	case v < 0x80:
		return append(dst, byte(v))
	case v < 0x4000:
		return append(dst, byte(0x80|(v&0x7f)), byte(v>>7))
	default:
		return append(dst, byte(0x80|(v&0x7f)), byte(0x80|((v>>7)&0x7f)), byte(v>>14))
	}
}

func DecodeB64(s string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("base64: %w", err)
	}
	return b, nil
}

func PublicFromSeed(seed []byte) ([]byte, error) {
	if len(seed) != 32 {
		return nil, errors.New("pubkey: seed must be 32 bytes")
	}
	k := ed25519.NewKeyFromSeed(seed)
	return k.Public().(ed25519.PublicKey), nil
}

func SignVersionedTx(msg []byte, k Keypair) []byte {

	vmsg := make([]byte, 0, len(msg)+2)
	vmsg = append(vmsg, 0x80)
	vmsg = append(vmsg, msg...)
	vmsg = append(vmsg, 0)
	sig := k.Sign(vmsg)
	out := []byte{1}
	out = append(out, sig[:]...)
	out = append(out, vmsg...)
	return out
}

func SignVersionedTxMulti(msg []byte, keys ...Keypair) ([]byte, error) {
	if len(msg) < 3 {
		return nil, errors.New("tx: legacy message too short")
	}
	signedCount := int(msg[0]) + int(msg[1])
	rawKeys, n, err := readKeys(msg[3:])
	if err != nil {
		return nil, err
	}
	if signedCount > n {
		return nil, fmt.Errorf("tx: header says %d signers, message has %d keys", signedCount, n)
	}
	byPub := map[string]Keypair{}
	for _, k := range keys {
		byPub[k.PublicBase58()] = k
	}
	vmsg := make([]byte, 0, len(msg)+2)
	vmsg = append(vmsg, 0x80)
	vmsg = append(vmsg, msg...)
	vmsg = append(vmsg, 0)
	sigs := make([]byte, 0, signedCount*64)
	for i := 0; i < signedCount; i++ {
		pub := Encode(rawKeys[i*32 : (i+1)*32])
		k, ok := byPub[pub]
		if !ok {
			return nil, fmt.Errorf("tx: missing signer %s", pub)
		}
		s := k.Sign(vmsg)
		sigs = append(sigs, s[:]...)
	}
	out := []byte{byte(signedCount)}
	out = append(out, sigs...)
	out = append(out, vmsg...)
	return out, nil
}

func readKeys(b []byte) ([]byte, int, error) {
	var shift uint
	n := 0
	for i := 0; i < 3; i++ {
		if i >= len(b) {
			return nil, 0, errors.New("tx: truncated compact u16")
		}
		c := b[i]
		n |= int(c&0x7f) << shift
		if c&0x80 == 0 {
			b = b[i+1:]
			goto keys
		}
		shift += 7
	}
	return nil, 0, errors.New("tx: compact u16 too long")
keys:
	total := n * 32
	if len(b) < total {
		return nil, 0, errors.New("tx: truncated key list")
	}
	return b[:total], n, nil
}

package sol

import (
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"math/big"
)

var p = func() *big.Int {

	n := new(big.Int).Lsh(big.NewInt(1), 255)
	n.Sub(n, big.NewInt(19))
	return n
}()

var curveD = func() *big.Int {

	num := new(big.Int).SetInt64(-121665)
	num.Mod(num, p)
	den := new(big.Int).SetInt64(121666)
	den.ModInverse(den, p)
	d := new(big.Int).Mul(num, den)
	return d.Mod(d, p)
}()

var sqrtM1 = func() *big.Int {

	e := new(big.Int).Sub(p, big.NewInt(1))
	e.Div(e, big.NewInt(4))
	return new(big.Int).Exp(big.NewInt(2), e, p)
}()

func IsOnCurve(b []byte) bool {
	if len(b) != 32 {
		return false
	}

	le := make([]byte, 32)
	copy(le, b)
	le[31] &= 0x7f
	rev := make([]byte, 32)
	for i := range le {
		rev[31-i] = le[i]
	}
	y := new(big.Int).SetBytes(rev)
	y.Mod(y, p)

	y2 := new(big.Int).Mul(y, y)
	y2.Mod(y2, p)
	num := new(big.Int).Sub(y2, big.NewInt(1))
	num.Mod(num, p)
	den := new(big.Int).Mul(curveD, y2)
	den.Add(den, big.NewInt(1))
	den.Mod(den, p)
	if den.Sign() == 0 {
		return false
	}
	denInv := new(big.Int).ModInverse(den, p)
	if denInv == nil {
		return false
	}
	x2 := new(big.Int).Mul(num, denInv)
	x2.Mod(x2, p)

	e := new(big.Int).Add(p, big.NewInt(3))
	e.Div(e, big.NewInt(8))
	x := new(big.Int).Exp(x2, e, p)
	x2check := new(big.Int).Mul(x, x)
	x2check.Mod(x2check, p)
	if x2check.Cmp(x2) != 0 {
		x.Mul(x, sqrtM1)
		x.Mod(x, p)
		x2check.Mul(x, x)
		x2check.Mod(x2check, p)
		if x2check.Cmp(x2) != 0 {
			return false
		}
	}

	if x.Sign() == 0 && b[31]&0x80 != 0 {
		return false
	}
	return true
}

func PDA(program string, seeds ...[]byte) (string, byte, error) {
	prog, err := Decode(program)
	if err != nil {
		return "", 0, err
	}
	if len(prog) != 32 {
		return "", 0, errors.New("pda: program id must be 32 bytes")
	}
	for nonce := 255; nonce > 0; nonce-- {
		b := byte(nonce)
		parts := append(append([][]byte{}, seeds...), []byte{b})
		parts = append(parts, prog, []byte("ProgramDerivedAddress"))
		h := hashv(parts...)
		if !IsOnCurve(h) {
			return Encode(h), b, nil
		}
	}
	return "", 0, errors.New("pda: no off-curve address found")
}

func hashv(parts ...[]byte) []byte {
	h := sha256.New()
	for _, part := range parts {
		h.Write(part)
	}
	return h.Sum(nil)
}

func pubOf(seed []byte) ed25519.PublicKey {
	k := ed25519.NewKeyFromSeed(seed)
	return k.Public().(ed25519.PublicKey)
}

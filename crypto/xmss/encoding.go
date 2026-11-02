package xmss

// Encoding.lean: the target-sum code of WOTS+C (ePrint 2022/778).
//
// A 16-byte digest D has 64 digits of 2 bits each:
//
//	x_i = floor(LE^-1(D) / 4^i) mod 4,   i < 64
//
// D is a codeword when its last 6 digits are zero and its first 58 sum to T = 106.

// digitOf returns digit i of a digest, the base-4 digit of weight 4^i of LE^-1(d).
// Digit i is bits 2i and 2i + 1 of the little-endian integer, so it is in byte i / 4.
func digitOf(d *digest, i int) uint8 {
	return (d[i/4] >> (2 * (i % 4))) & (chainLength - 1)
}

// codewordOf returns the codeword a digest is, as the digits of its 58 chains.
// It returns false when the digest is not a codeword.
func codewordOf(d digest) (codeword, bool) {
	var x codeword
	sum := 0
	for i := range numChains {
		x[i] = digitOf(&d, i)
		sum += int(x[i])
	}
	// Digits 58 to 63 have no chain, so a codeword sets them to zero.
	for i := numChains; i < numDigits; i++ {
		if digitOf(&d, i) != 0 {
			return codeword{}, false
		}
	}
	if sum != targetSum {
		return codeword{}, false
	}
	return x, true
}

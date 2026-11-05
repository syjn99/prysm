package xmss

import "encoding/binary"

// Address.lean: addresses keep the hash inputs of a key distinct.

// Address types: the type code tau of the note.
const (
	addrTypePrf   = 0  // The start of two chains, from the seed.
	addrTypeChain = 1  // One step along a chain.
	addrTypeLeaf  = 2  // A one-time key's Merkle leaf, from its chain ends.
	addrTypeNode  = 3  // A Merkle parent, from its two children.
	addrTypeParam = 5  // The public parameter, from the seed.
	addrTypeRnd   = 7  // The base of a one-time signature's randomizers.
	addrTypeMsg   = 12 // The digest a one-time key signs.
)

// encodeAddress returns enc(tau, mu, hi, lo), the 16 bytes of one address:
//
//	[ lo: 4 | hi: 3 | tau + 32 mu: 1 | zero: 8 ]
//
// tau < 32 is the address type, mu < 8 a chain position, hi < 2^24. Every field is
// little-endian. The caller keeps each argument inside its range.
func encodeAddress(tau, mu uint8, hi, lo uint32) address {
	var a address
	binary.LittleEndian.PutUint32(a[0:4], lo)
	a[4] = byte(hi)
	a[5] = byte(hi >> 8)
	a[6] = byte(hi >> 16)
	a[7] = tau + 32*mu
	return a
}

// layerHi returns hi_l(a) = a + 2^23 l. The top layer sets bit 23 of hi.
// The caller keeps a below 2^23.
func layerHi(l layer, a uint32) uint32 {
	return a + uint32(l)<<23
}

// addrPrf returns A_prf(l, e, j): the start of chains 2j and 2j + 1 of key e.
func addrPrf(l layer, e uint32, j uint32) address {
	return encodeAddress(addrTypePrf, 0, layerHi(l, j), e)
}

// addrChain returns A_chain(l, e, mu): the step from position mu of every chain of key e.
func addrChain(l layer, e uint32, mu uint8) address {
	return encodeAddress(addrTypeChain, mu, layerHi(l, 0), e)
}

// addrLeaf returns A_leaf(l, e): the Merkle leaf of key e.
func addrLeaf(l layer, e uint32) address {
	return encodeAddress(addrTypeLeaf, 0, layerHi(l, 0), e)
}

// addrNode returns A_node(l, level, j): node j on a level of a layer.
func addrNode(l layer, level uint32, j uint32) address {
	return encodeAddress(addrTypeNode, 0, layerHi(l, level), j)
}

// addrParam returns A_param: the public parameter.
func addrParam() address {
	return encodeAddress(addrTypeParam, 0, 0, 0)
}

// addrRnd returns A_rnd(l, e): the randomizer base of key e.
func addrRnd(l layer, e uint32) address {
	return encodeAddress(addrTypeRnd, 0, layerHi(l, 0), e)
}

// addrMsg returns A_msg(l, e): the digest key e signs.
func addrMsg(l layer, e uint32) address {
	return encodeAddress(addrTypeMsg, 0, layerHi(l, 0), e)
}

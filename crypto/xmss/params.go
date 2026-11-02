// Package xmss implements leanXMSS, the hash-based signature scheme of EIP-8450.
//
// It is a port of the Lean model in ethereum/EthLib, EthLib/Cryptography/Xmss, at commit
// 6390c73ef040311e807b9047d99dca6b1be1c42f. The Lean model follows the note "leanXMSS for
// Ethereum PQ-Consensus, specification draft". Each file names the Lean module it ports.
//
// A key is two layers of Merkle trees of height 16, whose leaves are WOTS+C one-time keys.
// The top layer is one tree, and its root is the public key. The bottom layer is 2^16
// trees, and top key b signs the root of bottom tree b. So a key signs at 2^32 leaf
// indices, each one at most once.
//
// The hash is Keccak-256. A public key is 32 bytes and a signature is 2404 bytes.
//
// The package and its tests import only the standard library and golang.org/x/crypto/sha3,
// so the directory can be copied out of Prysm as is.
package xmss

// Constants.lean: the leanXMSS parameter set.
const (
	// digestLen is the hash value and Merkle node length in bytes, n in the note.
	digestLen = 16
	// publicParamLen is the public parameter length in bytes.
	publicParamLen = 16
	// addressLen is the address length in bytes.
	addressLen = 16
	// messageLen is the message length in bytes: a 256-bit message hash.
	messageLen = 32
	// randomizerLen is the randomizer length in bytes.
	randomizerLen = 16
	// seedLen is the secret key length in bytes.
	seedLen = 32

	// chainLength is the number of positions of one hash chain, 0 to 3.
	chainLength = 4
	// numDigits is the number of 2-bit digits in a 16-byte digest.
	numDigits = 64
	// numChains is the number of chains of one one-time key, v in the note.
	numChains = 58
	// targetSum is the sum of the digits of a codeword, T in the note.
	targetSum = 106
	// maxRandomizerTrials is the number of randomizer trials per one-time signature, A_max.
	maxRandomizerTrials = 1 << 32

	// layerHeight is the height of the trees of each layer, h in the note.
	layerHeight = 16

	// pubKeySize is the size of a serialized public key: the root, then the public parameter.
	pubKeySize = digestLen + publicParamLen
	// leafIndexSize is the size of a serialized leaf index, a little-endian 32-bit word.
	leafIndexSize = 4
	// layerSigSize is the size of one layer's signature: randomizer, chain values, path.
	layerSigSize = randomizerLen + numChains*digestLen + layerHeight*digestLen
	// sigSize is the size of a serialized signature: the leaf index, then the bottom and top layers.
	sigSize = leafIndexSize + 2*layerSigSize
)

// seedTag is the scheme's name, hashed in front of the seed: ASCII "leanXMSS".
var seedTag = []byte("leanXMSS")

// Types.lean: fixed-size byte objects.
type (
	// digest is a chain value, a Merkle node or a leaf.
	digest = [digestLen]byte
	// publicParam is the public parameter P, hashed into every call.
	publicParam = [publicParamLen]byte
	// address is the address A that names the call site of one hash.
	address = [addressLen]byte
	// randomizer is the randomizer rho under which a digest is a codeword.
	randomizer = [randomizerLen]byte
	// codeword holds the digits of the 58 chains of a codeword, each below chainLength.
	codeword = [numChains]uint8
)

// layer is one of the two layers of trees. The value is the layer number l of the note.
type layer uint8

const (
	// bottomLayer has 2^16 trees, whose keys sign messages.
	bottomLayer layer = 0
	// topLayer has one tree, whose keys sign the roots of the bottom trees.
	topLayer layer = 1
)

// bottomTree returns the bottom tree of a leaf index: b = floor(idx / 2^16).
// It is also the index of the top key that signs the root of that tree.
func bottomTree(idx uint32) uint32 {
	return idx >> layerHeight
}

package beacon

import (
	"fmt"
	"os"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	sszquerypb "github.com/OffchainLabs/prysm/v6/proto/ssz_query"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

const (
	stateFilePath  = "./state_12852789.ssz"
	validatorIndex = 2045754
	targetPath     = ".validators[2045754]"
)

// Preloaded variables that are shared across benchmarks.
var (
	preloadedState        state.BeaconState
	preloadedEncodedState []byte
	preloadedInfo         *query.SszInfo
	preloadedPath         []query.PathElement
	preloadedRoot         [32]byte

	// global values are used to prevent the compiler from optimizing
	// away the function calls we are trying to measure.
	globalResult     []byte
	globalProtoState any
)

// BenchmarkQueryBeaconState benchmarks the end-to-end SSZ Query handling,
// given a preloaded BeaconState.
// Note: Fetching a state won't be part of this benchmark.
// Note 2: See other benchmarks for measuring individual steps.
func BenchmarkQueryBeaconState(b *testing.B) {
	setupBenchmark(b)

	st := preloadedState
	path := preloadedPath
	root := preloadedRoot

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sszObject, _ := st.ToProtoUnsafe().(query.SSZObject)

		info, err := query.AnalyzeObject(sszObject)
		require.NoError(b, err)

		_, offset, length, err := query.CalculateOffsetAndLength(info, path)
		require.NoError(b, err)

		encodedState, err := st.MarshalSSZ()
		require.NoError(b, err)

		response := &sszquerypb.SSZQueryResponse{
			Root:   root[:],
			Result: encodedState[offset : offset+length],
		}
		responseSsz, err := response.MarshalSSZ()
		require.NoError(b, err)

		globalResult = responseSsz
	}
}

// 1. Benchmark the ToProtoUnsafe step in isolation.
func Benchmark_A_ToProtoUnsafe(b *testing.B) {
	setupBenchmark(b)

	st := preloadedState

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		protoState := st.ToProtoUnsafe()
		globalProtoState = protoState
	}
}

// 2. Benchmark the AnalyzeObject step in isolation.
func Benchmark_B_AnalyzeObject(b *testing.B) {
	setupBenchmark(b)

	sszObject, _ := preloadedState.ToProtoUnsafe().(query.SSZObject)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		info, err := query.AnalyzeObject(sszObject)
		require.NoError(b, err)

		// Prevent optimization...
		if info.Size() == 0 {
			b.Fatal("dummy check")
		}
	}
}

// 3. Benchmark the CalculateOffsetAndLength in isolation.
func Benchmark_C_CalculateOffsetAndLength(b *testing.B) {
	setupBenchmark(b)

	info := preloadedInfo
	path := preloadedPath
	encodedState := preloadedEncodedState

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, offset, length, err := query.CalculateOffsetAndLength(info, path)
		require.NoError(b, err)

		globalResult = encodedState[offset : offset+length]
	}
}

// 4. Benchmark the MarshalSSZ step in isolation.
func Benchmark_D_MarshalSSZ_BeaconState(b *testing.B) {
	setupBenchmark(b)
	st := preloadedState

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encodedState, err := st.MarshalSSZ()
		require.NoError(b, err)

		globalResult = encodedState
	}
}

// 5. Benchmark the MarshalSSZ step for the response in isolation.
func Benchmark_E_MarshalSSZ_SSZQueryResponse(b *testing.B) {
	setupBenchmark(b)

	st := preloadedState
	path := preloadedPath
	root := preloadedRoot

	_, offset, length, err := query.CalculateOffsetAndLength(preloadedInfo, path)
	require.NoError(b, err)

	encodedState, err := st.MarshalSSZ()
	require.NoError(b, err)

	resultSlice := encodedState[offset : offset+length]

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		response := &sszquerypb.SSZQueryResponse{
			Root:   root[:],
			Result: resultSlice,
		}
		responseSsz, err := response.MarshalSSZ()
		require.NoError(b, err)

		globalResult = responseSsz
	}
}

// One-time setup for benchmarks.
// Needed for measuring each step in isolation.
func setupBenchmark(b *testing.B) {
	if preloadedState != nil {
		return
	}

	ctx := b.Context()

	b.Log("Running one-time setup...")

	// 1. Load the SSZ file
	var err error
	preloadedEncodedState, err = os.ReadFile(stateFilePath)
	require.NoError(b, err)

	// 2. Unmarshal it into the Go struct, and then into the native state
	electraState := new(ethpb.BeaconStateElectra)
	err = electraState.UnmarshalSSZ(preloadedEncodedState)
	require.NoError(b, err)

	preloadedState, err = state_native.InitializeFromProtoElectra(electraState)
	require.NoError(b, err)

	// 3. Pre-calculate the root
	preloadedRoot, err = preloadedState.HashTreeRoot(ctx)
	require.NoError(b, err)

	// 4. Pre-calculate the SszInfo tree
	sszObject, ok := preloadedState.ToProtoUnsafe().(query.SSZObject)
	require.Equal(b, ok, true)

	// 5. Pre-analyze the object
	preloadedInfo, err = query.AnalyzeObject(sszObject)
	require.NoError(b, err)

	// 6. Pre-parse a realistic query path
	preloadedPath, err = query.ParsePath(targetPath)
	require.NoError(b, err)

	b.Log("Setup complete.")
}

func TestCompareResult(t *testing.T) {
	var err error
	encodedState, err := os.ReadFile(stateFilePath)
	require.NoError(t, err)

	electraState := new(ethpb.BeaconStateElectra)
	err = electraState.UnmarshalSSZ(encodedState)
	require.NoError(t, err)

	st, err := state_native.InitializeFromProtoElectra(electraState)
	require.NoError(t, err)

	sszObject, ok := st.ToProtoUnsafe().(query.SSZObject)
	require.Equal(t, ok, true)

	info, err := query.AnalyzeObject(sszObject)
	require.NoError(t, err)

	path, err := query.ParsePath(targetPath)
	require.NoError(t, err)

	_, offset, length, err := query.CalculateOffsetAndLength(info, path)
	require.NoError(t, err)

	resultBytes := encodedState[offset : offset+length]
	require.NotEmpty(t, resultBytes)

	result := new(ethpb.Validator)
	err = result.UnmarshalSSZ(resultBytes)
	require.NoError(t, err)

	targetValidator, err := st.ValidatorAtIndex(validatorIndex)
	require.NoError(t, err)

	require.DeepEqual(t, targetValidator, result)
}

// An utility test to print out the SszInfo tree.
func TestPrintSszInfo(t *testing.T) {
	var err error
	encodedState, err := os.ReadFile(stateFilePath)
	require.NoError(t, err)

	electraState := new(ethpb.BeaconStateElectra)
	err = electraState.UnmarshalSSZ(encodedState)
	require.NoError(t, err)

	st, err := state_native.InitializeFromProtoElectra(electraState)
	require.NoError(t, err)

	sszObject, ok := st.ToProtoUnsafe().(query.SSZObject)
	require.Equal(t, ok, true)

	info, err := query.AnalyzeObject(sszObject)
	require.NoError(t, err)

	fmt.Println(info.Print())
}

package epoch_processing

import (
	"context"
	"path"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/epoch"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/epoch/precompute"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/spectest/utils"
)

// RunSlashingsTests executes "epoch_processing/slashings" tests.
func RunSlashingsTests(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))

	testFolders, testsFolderPath := utils.TestFolders(t, config, "phase0", "epoch_processing/slashings/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, "phase0", "epoch_processing/slashings/pyspec_tests")
	}
	for _, folder := range testFolders {
		helpers.ClearCache()
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			RunEpochOperationTest(t, folderPath, processSlashingsWrapper)
			RunEpochOperationTest(t, folderPath, processSlashingsPrecomputeWrapper)
		})
	}
}

func processSlashingsWrapper(t *testing.T, st state.BeaconState) (state.BeaconState, error) {
	require.NoError(t, epoch.ProcessSlashings(st), "Could not process slashings")
	return st, nil
}

func processSlashingsPrecomputeWrapper(t *testing.T, state state.BeaconState) (state.BeaconState, error) {
	ctx := context.Background()
	vp, bp, err := precompute.New(ctx, state)
	require.NoError(t, err)
	_, bp, err = precompute.ProcessAttestations(ctx, state, vp, bp)
	require.NoError(t, err)

	return state, precompute.ProcessSlashingsPrecompute(state, bp)
}

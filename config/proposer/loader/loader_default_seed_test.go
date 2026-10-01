package loader

import (
	"flag"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/urfave/cli/v2"

	"github.com/OffchainLabs/prysm/v7/cmd/validator/flags"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/db/iface"
	dbTest "github.com/OffchainLabs/prysm/v7/validator/db/testing"
)

const droppedGasLimitLog = "Dropped the default gas limit"

var seedFeeRecipient = common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A")

// seedCliCtx builds a context with --suggested-gas-limit and/or --builder-min-bid=1 set; "" / false leaves a flag unset.
func seedCliCtx(t *testing.T, gasLimit string, minBid bool) *cli.Context {
	set := flag.NewFlagSet("x", flag.ContinueOnError)
	set.String(flags.BuilderGasLimitFlag.Name, "", "")
	set.Uint64(flags.BuilderMinBidFlag.Name, 0, "")
	if gasLimit != "" {
		require.NoError(t, set.Set(flags.BuilderGasLimitFlag.Name, gasLimit))
	}
	if minBid {
		require.NoError(t, set.Set(flags.BuilderMinBidFlag.Name, "1"))
	}
	return cli.NewContext(&cli.App{}, set, nil)
}

func seedLoad(t *testing.T, db iface.ValidatorDB, cliCtx *cli.Context) *proposer.Settings {
	l, err := NewProposerSettingsLoader(cliCtx, db, WithBuilderConfig(), WithGasLimit())
	require.NoError(t, err)
	ps, err := l.Load(cliCtx)
	require.NoError(t, err)
	return ps
}

func TestLoadFromDefault_SeedsDBBuilder(t *testing.T) {
	var key [fieldparams.BLSPubkeyLength]byte
	chainDefault := validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit)
	seed := func(t *testing.T, version uint32) iface.ValidatorDB {
		db := dbTest.SetupDB(t, t.TempDir(), nil, false)
		require.NoError(t, db.SaveProposerSettings(t.Context(), &proposer.Settings{
			Version: version,
			DefaultConfig: &proposer.Option{
				FeeRecipientConfig: &proposer.FeeRecipientConfig{FeeRecipient: seedFeeRecipient},
				BuilderConfig:      &proposer.BuilderConfig{Enabled: true},
			},
		}))
		return db
	}

	for _, tc := range []struct {
		name     string
		gasLimit string
		minBid   bool
		wantGas  validator.Uint64
	}{
		{name: "flagless", wantGas: chainDefault},
		{name: "suggested gas limit only", gasLimit: "40000000", wantGas: 40000000},
		{name: "builder min bid only", minBid: true, wantGas: chainDefault},
	} {
		t.Run("v2 DB keeps default builder.enabled/"+tc.name, func(t *testing.T) {
			db := seed(t, proposer.SchemaV2)
			ps := seedLoad(t, db, seedCliCtx(t, tc.gasLimit, tc.minBid))
			fr, gas, enabled := ps.RegistrationFor(key)
			assert.Equal(t, seedFeeRecipient, fr)
			assert.Equal(t, true, enabled)
			assert.Equal(t, tc.wantGas, gas)
			assert.Equal(t, tc.wantGas, ps.GasLimit(key))
			if tc.minBid {
				require.NotNil(t, ps.DefaultConfig.BuilderConfig.MinBid)
				assert.Equal(t, validator.Uint64(1), *ps.DefaultConfig.BuilderConfig.MinBid)
			}

			// The toggle is persisted, so a flagless restart still registers.
			ps = seedLoad(t, db, seedCliCtx(t, "", false))
			fr, gas, enabled = ps.RegistrationFor(key)
			assert.Equal(t, seedFeeRecipient, fr)
			assert.Equal(t, true, enabled)
			assert.Equal(t, chainDefault, gas)
		})
	}

	t.Run("v1 DB still strips the builder without --enable-builder", func(t *testing.T) {
		db := seed(t, proposer.SchemaV1Unset)
		ps := seedLoad(t, db, seedCliCtx(t, "40000000", false))
		fr, gas, enabled := ps.RegistrationFor(key)
		assert.Equal(t, seedFeeRecipient, fr)
		assert.Equal(t, false, enabled)
		assert.Equal(t, validator.Uint64(40000000), gas)
	})
}

func TestLoad_GasLimitOnlyDefaultNotPersisted(t *testing.T) {
	var key [fieldparams.BLSPubkeyLength]byte
	chainDefault := validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit)

	t.Run("fresh DB, gas limit only", func(t *testing.T) {
		db := dbTest.SetupDB(t, t.TempDir(), nil, false)
		ps := seedLoad(t, db, seedCliCtx(t, "40000000", false))
		assert.Equal(t, validator.Uint64(40000000), ps.GasLimit(key))
		exists, err := db.ProposerSettingsExists(t.Context())
		require.NoError(t, err)
		assert.Equal(t, false, exists)

		for range 2 {
			hook := logTest.NewGlobal()
			seedLoad(t, db, seedCliCtx(t, "", false))
			require.LogsDoNotContain(t, hook, droppedGasLimitLog)
		}
	})

	t.Run("v2 DB with fee recipient keeps the gas limit and warns once", func(t *testing.T) {
		db := dbTest.SetupDB(t, t.TempDir(), nil, false)
		require.NoError(t, db.SaveProposerSettings(t.Context(), &proposer.Settings{
			Version: proposer.SchemaV2,
			DefaultConfig: &proposer.Option{
				FeeRecipientConfig: &proposer.FeeRecipientConfig{FeeRecipient: seedFeeRecipient},
			},
		}))
		seedLoad(t, db, seedCliCtx(t, "40000000", false))
		stored, err := db.ProposerSettings(t.Context())
		require.NoError(t, err)
		assert.Equal(t, validator.Uint64(40000000), stored.DefaultConfig.GasLimit)
		assert.Equal(t, seedFeeRecipient, stored.DefaultConfig.FeeRecipientConfig.FeeRecipient)

		hook := logTest.NewGlobal()
		ps := seedLoad(t, db, seedCliCtx(t, "", false))
		require.LogsContain(t, hook, droppedGasLimitLog)
		assert.Equal(t, chainDefault, ps.GasLimit(key))

		hook = logTest.NewGlobal()
		seedLoad(t, db, seedCliCtx(t, "", false))
		require.LogsDoNotContain(t, hook, droppedGasLimitLog)
	})
}

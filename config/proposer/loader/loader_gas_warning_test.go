package loader

import (
	"flag"
	"testing"

	"github.com/OffchainLabs/prysm/v7/cmd/validator/flags"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	dbTest "github.com/OffchainLabs/prysm/v7/validator/db/testing"
	"github.com/ethereum/go-ethereum/common"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/urfave/cli/v2"
)

func TestDefaultGasLimitDropWarning(t *testing.T) {
	const warning = "Dropped the default gas limit"
	const gasLimit = validator.Uint64(45000000)
	const feeRecipient = "0x6e35733c5af9B61374A128e6F85f553aF09ff89A"
	key := [fieldparams.BLSPubkeyLength]byte{}

	for _, tc := range []struct {
		name        string
		version     uint32
		enable      bool
		wantWarning bool
	}{
		{name: "v1 legacy builder gas remains active", enable: true},
		{name: "v2 option gas is dropped", version: proposer.SchemaV2, wantWarning: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := dbTest.SetupDB(t, t.TempDir(), nil, false)
			if tc.version != 0 {
				require.NoError(t, db.SaveProposerSettings(t.Context(), &proposer.Settings{
					Version: tc.version,
					DefaultConfig: &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{FeeRecipient: common.HexToAddress(feeRecipient)},
					},
				}))
			}

			load := func(withGas bool) *proposer.Settings {
				set := flag.NewFlagSet("test", 0)
				if withGas {
					set.String(flags.SuggestedFeeRecipientFlag.Name, "", "")
					require.NoError(t, set.Set(flags.SuggestedFeeRecipientFlag.Name, feeRecipient))
					set.String(flags.BuilderGasLimitFlag.Name, "", "")
					require.NoError(t, set.Set(flags.BuilderGasLimitFlag.Name, "45000000"))
				}
				if tc.enable {
					set.Bool(flags.EnableBuilderFlag.Name, false, "")
					require.NoError(t, set.Set(flags.EnableBuilderFlag.Name, "true"))
				}
				ctx := cli.NewContext(&cli.App{}, set, nil)
				loader, err := NewProposerSettingsLoader(ctx, db, WithBuilderConfig(), WithGasLimit())
				require.NoError(t, err)
				settings, err := loader.Load(ctx)
				require.NoError(t, err)
				require.NotNil(t, settings)
				return settings
			}

			first := load(true)
			_, firstGas, _ := first.RegistrationFor(key)
			require.Equal(t, gasLimit, firstGas)

			hook := logTest.NewGlobal()
			second := load(false)
			_, secondGas, enabled := second.RegistrationFor(key)
			if tc.wantWarning {
				require.LogsContain(t, hook, warning)
				require.Equal(t, validator.Uint64(0), second.DefaultConfig.GasLimit)
				require.Equal(t, validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit), secondGas)
				require.Equal(t, false, enabled)
			} else {
				require.LogsDoNotContain(t, hook, warning)
				require.Equal(t, gasLimit, secondGas)
				require.Equal(t, true, enabled)
			}
		})
	}
}

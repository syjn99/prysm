package proposer_test

import (
	"flag"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/urfave/cli/v2"

	"github.com/OffchainLabs/prysm/v7/cmd/validator/flags"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/config/proposer/loader"
	"github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	dbTest "github.com/OffchainLabs/prysm/v7/validator/db/testing"
)

func TestGasLimitPerKeyOutranksDefault(t *testing.T) {
	pk := [fieldparams.BLSPubkeyLength]byte{0xaa}
	feeRecipient := &proposer.FeeRecipientConfig{FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A")}

	t.Run("resolver order", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			perKey *proposer.Option
			def    *proposer.Option
			want   validator.Uint64
		}{
			{"per-key option beats per-key builder", &proposer.Option{GasLimit: 1, BuilderConfig: &proposer.BuilderConfig{GasLimit: 2}}, &proposer.Option{GasLimit: 3}, 1},
			{"per-key builder beats default option", &proposer.Option{BuilderConfig: &proposer.BuilderConfig{GasLimit: 2}}, &proposer.Option{GasLimit: 3, BuilderConfig: &proposer.BuilderConfig{GasLimit: 4}}, 2},
			{"default option beats default builder", nil, &proposer.Option{GasLimit: 3, BuilderConfig: &proposer.BuilderConfig{GasLimit: 4}}, 3},
			{"default builder is the last fallback", nil, &proposer.Option{BuilderConfig: &proposer.BuilderConfig{GasLimit: 4}}, 4},
		} {
			t.Run(tc.name, func(t *testing.T) {
				def := tc.def
				def.FeeRecipientConfig = feeRecipient
				ps := &proposer.Settings{DefaultConfig: def}
				if tc.perKey != nil {
					ps.ProposeConfig = map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{pk: tc.perKey}
				}
				require.Equal(t, tc.want, ps.GasLimit(pk))
				_, gas, _ := ps.RegistrationFor(pk)
				require.Equal(t, tc.want, gas)
			})
		}
	})

	t.Run("v2 per-key builder gas limit beats --suggested-gas-limit", func(t *testing.T) {
		db := dbTest.SetupDB(t, t.TempDir(), nil, false)
		require.NoError(t, db.SaveProposerSettings(t.Context(), &proposer.Settings{
			Version: proposer.SchemaV2,
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
				pk: {BuilderConfig: &proposer.BuilderConfig{Enabled: true, GasLimit: 25000000}},
			},
			DefaultConfig: &proposer.Option{FeeRecipientConfig: feeRecipient},
		}))
		set := flag.NewFlagSet("x", flag.ContinueOnError)
		set.String(flags.BuilderGasLimitFlag.Name, "", "")
		require.NoError(t, set.Set(flags.BuilderGasLimitFlag.Name, "40000000"))
		cliCtx := cli.NewContext(&cli.App{}, set, nil)
		l, err := loader.NewProposerSettingsLoader(cliCtx, db, loader.WithBuilderConfig(), loader.WithGasLimit())
		require.NoError(t, err)
		ps, err := l.Load(cliCtx)
		require.NoError(t, err)

		require.Equal(t, validator.Uint64(25000000), ps.GasLimit(pk))
		_, gas, on := ps.RegistrationFor(pk)
		require.Equal(t, true, on)
		require.Equal(t, validator.Uint64(25000000), gas)
		other := [fieldparams.BLSPubkeyLength]byte{0xbb}
		require.Equal(t, validator.Uint64(40000000), ps.GasLimit(other), "keys without their own gas limit take the flag")
	})
}

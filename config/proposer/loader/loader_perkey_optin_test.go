package loader

import (
	"flag"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/urfave/cli/v2"

	"github.com/OffchainLabs/prysm/v7/cmd/validator/flags"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/db/iface"
	dbTest "github.com/OffchainLabs/prysm/v7/validator/db/testing"
)

func TestBuilderURLsPerKeyOptIn(t *testing.T) {
	legacyOff := [fieldparams.BLSPubkeyLength]byte{0xaa}
	optOut := [fieldparams.BLSPubkeyLength]byte{0xbb}
	legacyOn := [fieldparams.BLSPubkeyLength]byte{0xcc}
	feeRecipient := common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A")

	load := func(t *testing.T, db iface.ValidatorDB, builderURLs string) *proposer.Settings {
		set := flag.NewFlagSet("x", flag.ContinueOnError)
		if builderURLs != "" {
			set.Var(cli.NewStringSlice(), flags.BuilderURLsFlag.Name, "")
			require.NoError(t, set.Set(flags.BuilderURLsFlag.Name, builderURLs))
		}
		cliCtx := cli.NewContext(&cli.App{}, set, nil)
		l, err := NewProposerSettingsLoader(cliCtx, db, WithBuilderConfig(), WithGasLimit())
		require.NoError(t, err)
		ps, err := l.Load(cliCtx)
		require.NoError(t, err)
		return ps
	}

	for _, minimal := range []bool{false, true} {
		t.Run(fmt.Sprintf("builder-urls opts legacy-only keys in for that run only/minimal:%v", minimal), func(t *testing.T) {
			db := dbTest.SetupDB(t, t.TempDir(), nil, minimal)
			require.NoError(t, db.SaveProposerSettings(t.Context(), &proposer.Settings{
				Version: proposer.SchemaV2,
				ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
					legacyOff: {BuilderConfig: &proposer.BuilderConfig{GasLimit: 30000000}},
					optOut:    {BuilderConfig: &proposer.BuilderConfig{Builders: []*proposer.BuilderEntry{}}},
					legacyOn:  {BuilderConfig: &proposer.BuilderConfig{Enabled: true, GasLimit: 30000000}},
				},
				DefaultConfig: &proposer.Option{FeeRecipientConfig: &proposer.FeeRecipientConfig{FeeRecipient: feeRecipient}},
			}))

			ps := load(t, db, "https://builder.example")
			_, _, on := ps.RegistrationFor(legacyOff)
			require.Equal(t, true, on, "legacy-only key inherits the flag builders' registration")
			_, _, on = ps.RegistrationFor(optOut)
			require.Equal(t, false, on, "builders: [] still opts out")
			_, _, on = ps.RegistrationFor(legacyOn)
			require.Equal(t, true, on)
			require.Equal(t, false, ps.ProposeConfig[legacyOff].BuilderConfig.Enabled, "per-key block must not be rewritten")

			stored, err := db.ProposerSettings(t.Context())
			require.NoError(t, err)
			require.Equal(t, false, stored.ProposeConfig[legacyOff].BuilderConfig.Enabled)

			ps = load(t, db, "")
			_, _, on = ps.RegistrationFor(legacyOff)
			require.Equal(t, false, on, "a flagless run must not keep the previous run's opt-in")
			_, _, on = ps.RegistrationFor(legacyOn)
			require.Equal(t, true, on)
			stored, err = db.ProposerSettings(t.Context())
			require.NoError(t, err)
			require.Equal(t, false, stored.ProposeConfig[legacyOff].BuilderConfig.Enabled)
			require.Equal(t, validator.Uint64(30000000), ps.GasLimit(legacyOff))
		})
	}
}

package loader

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/cmd/validator/flags"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	dbTest "github.com/OffchainLabs/prysm/v7/validator/db/testing"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/urfave/cli/v2"
)

// --suggested-gas-limit fills in the default gas limit of a settings source that sets none.
func TestLoad_GasLimitFlagWithSettingsFile(t *testing.T) {
	// Gloas scheduled so the schedule-override warning is live.
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 100
	params.OverrideBeaconConfig(cfg)
	const flagGas = validator.Uint64(40_000_000)
	tests := []struct {
		name         string
		file         string
		wantGas      validator.Uint64
		wantReplaced bool
	}{
		{
			name:    "default_config without gas_limit takes the flag",
			file:    `{"version":2,"default_config":{"fee_recipient":"0x6e35733c5af9B61374A128e6F85f553aF09ff89A","builder":{"enabled":true}}}`,
			wantGas: flagGas,
		},
		{
			name:         "default_config gas_limit wins and the flag is reported replaced",
			file:         `{"version":2,"default_config":{"fee_recipient":"0x6e35733c5af9B61374A128e6F85f553aF09ff89A","gas_limit":"50000000","builder":{"enabled":true}}}`,
			wantGas:      50_000_000,
			wantReplaced: true,
		},
		{
			name:    "no default_config keeps the flag",
			file:    `{"version":2,"proposer_config":{"0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a":{"fee_recipient":"0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"}}}`,
			wantGas: flagGas,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			require.NoError(t, os.WriteFile(path, []byte(tt.file), 0o600))
			db := dbTest.SetupDB(t, t.TempDir(), nil, false)
			// Two runs on the same DB: the second merges onto what the first persisted.
			for run := 1; run <= 2; run++ {
				hook := logTest.NewGlobal()
				set := flag.NewFlagSet("test", 0)
				set.String(flags.ProposerSettingsFlag.Name, "", "")
				require.NoError(t, set.Set(flags.ProposerSettingsFlag.Name, path))
				set.String(flags.BuilderGasLimitFlag.Name, "", "")
				require.NoError(t, set.Set(flags.BuilderGasLimitFlag.Name, "40000000"))
				set.Bool(flags.EnableBuilderFlag.Name, false, "")
				require.NoError(t, set.Set(flags.EnableBuilderFlag.Name, "true"))
				cliCtx := cli.NewContext(&cli.App{}, set, nil)
				cliCtx.Context = context.Background()

				l, err := NewProposerSettingsLoader(cliCtx, db, WithBuilderConfig(), WithGasLimit())
				require.NoError(t, err)
				ps, err := l.Load(cliCtx)
				require.NoError(t, err)

				var key [fieldparams.BLSPubkeyLength]byte
				_, gas, _ := ps.RegistrationFor(key)
				require.Equal(t, tt.wantGas, gas, "run %d RegistrationFor", run)
				require.Equal(t, tt.wantGas, ps.GasLimit(key), "run %d GasLimit", run)
				if tt.wantReplaced {
					require.LogsContain(t, hook, "replaces the builder defaults set by --"+flags.BuilderGasLimitFlag.Name)
					assert.LogsDoNotContain(t, hook, "overrides the network gas limit schedule")
				} else {
					assert.LogsDoNotContain(t, hook, "replaces the builder defaults set by")
					assert.LogsContain(t, hook, "overrides the network gas limit schedule")
				}
			}
		})
	}
}

package accounts

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestImportPassword_NoWalletRequiresFile(t *testing.T) {
	acm := &CLIManager{}
	_, err := acm.importPassword()
	require.ErrorContains(t, "--account-password-file is required", err)

	pwFile := filepath.Join(t.TempDir(), "pw.txt")
	require.NoError(t, os.WriteFile(pwFile, []byte("hunter2\n"), 0600))
	acm = &CLIManager{readPasswordFile: true, passwordFilePath: pwFile}
	got, err := acm.importPassword()
	require.NoError(t, err)
	require.Equal(t, "hunter2", got)
}

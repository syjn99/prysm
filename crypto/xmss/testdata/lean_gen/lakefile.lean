import Lake
open Lake DSL

package «xmssgen» where

lean_lib «EthLib» where
  globs := #[.submodules `EthLib]

@[default_target]
lean_exe «gen» where
  root := `Main

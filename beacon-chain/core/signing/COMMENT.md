# beacon-chain/core/signing

- `domain.go` => `Domain`: Return domain like eth2 spec. Use `ComputeDomain` to actual computation.
- `signature.go` => Not using anymore... maybe legacy code that is used in validator client?
- `signing_root.go`
  - For domain and digest:
    - `ComputeDomain`: implementation of `compute_domain`.
    - `computeForkDataRoot`: implementation of `computeForkDataRoot`. Use to avoid collisions across forks/chains.
      - use `digestMap` for caching value, because this is not be computed often.
    - `ComputeForkDigest`: make data root to 4 bytes value.
  - For batching signatures:
    - `ComputeSigningRootForRoot`: Compute HTR for signing data container, which is:
      ```go
      container := &ethpb.SigningData{
    		ObjectRoot: root[:],
    		Domain:     domain,
    	}
      ``` 
    - `BlockSignatureBatch`: build struct(`SignatureBatch`), add description (`BlockSignature`)
    - and so on... verify block header, block itself, arbitrary signing data.
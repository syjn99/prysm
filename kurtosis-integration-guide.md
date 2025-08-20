# Prysm E2E to Kurtosis Integration Guide

## Executive Summary

This document outlines strategies for integrating Prysm's existing Bazel-based E2E tests (`//testing/endtoend:go_default_test`) with Kurtosis and the Ethereum Package. The goal is to leverage Kurtosis's infrastructure while maintaining Prysm's testing requirements and CI/CD compatibility.

## Integration Approaches

### Approach 1: Bazel + Kurtosis CLI (Recommended)

The most straightforward approach is to use Bazel to orchestrate Kurtosis via CLI commands, then run Assertoor tests against the deployed network.

#### Implementation Strategy

1. **Create a Bazel Rule for Kurtosis**
```python
# BUILD.bazel
load("@bazel_skylib//rules:run_binary.bzl", "run_binary")

# Custom rule to run Kurtosis
sh_test(
    name = "kurtosis_e2e_test",
    srcs = ["run_kurtosis_test.sh"],
    data = [
        "kurtosis_config.yaml",
        "assertoor_tests.yaml",
    ],
    deps = [
        "@kurtosis_cli//:kurtosis",  # Need to define kurtosis as external dependency
    ],
    tags = ["manual", "e2e"],
    size = "large",
    timeout = "long",  # E2E tests can take 30+ minutes
)
```

2. **Shell Script Wrapper** (`run_kurtosis_test.sh`)
```bash
#!/bin/bash
set -euo pipefail

# Configuration
ENCLAVE_NAME="prysm-e2e-${BUILD_ID:-local}"
KURTOSIS_CONFIG="${1:-kurtosis_config.yaml}"
CLEANUP_ON_EXIT="${CLEANUP_ON_EXIT:-true}"

# Cleanup function
cleanup() {
    if [[ "$CLEANUP_ON_EXIT" == "true" ]]; then
        echo "Cleaning up enclave: $ENCLAVE_NAME"
        kurtosis enclave rm -f "$ENCLAVE_NAME" || true
    fi
}
trap cleanup EXIT

# Start Kurtosis enclave
echo "Starting Kurtosis enclave: $ENCLAVE_NAME"
kurtosis run \
    --enclave "$ENCLAVE_NAME" \
    github.com/ethpandaops/ethereum-package \
    --args-file "$KURTOSIS_CONFIG"

# Wait for network to be ready
echo "Waiting for network finalization..."
sleep 60  # Initial wait for network startup

# Get service information
BEACON_HTTP_URL=$(kurtosis enclave inspect "$ENCLAVE_NAME" | grep -o 'http://.*:4000' | head -1)
export BEACON_HTTP_URL

# Run test verification
echo "Running test assertions..."
./verify_network.sh "$ENCLAVE_NAME"

# Check Assertoor test results
ASSERTOOR_URL=$(kurtosis enclave inspect "$ENCLAVE_NAME" | grep -o 'http://.*:8080' | head -1)
curl -s "${ASSERTOOR_URL}/api/v1/test_runs" | jq -e '.test_runs[0].status == "success"'
```

3. **Kurtosis Configuration** (`kurtosis_config.yaml`)
```yaml
# Minimal E2E test configuration matching Prysm's requirements
participants:
  - el_type: geth
    cl_type: prysm
    cl_image: "gcr.io/offchainlabs/prysm/beacon-chain:latest"
    vc_type: prysm
    vc_image: "gcr.io/offchainlabs/prysm/validator:latest"
    count: 2
    validator_count: 64

network_params:
  network: "kurtosis"
  seconds_per_slot: 12
  slots_per_epoch: 32  # Minimal preset for faster testing
  num_validator_keys_per_node: 64
  genesis_delay: 20
  altair_fork_epoch: 1
  bellatrix_fork_epoch: 2
  capella_fork_epoch: 3
  deneb_fork_epoch: 4
  electra_fork_epoch: 5

# Enable test services
additional_services:
  - assertoor
  - tx_fuzz
  - prometheus
  - grafana

# Assertoor configuration for Prysm evaluators
assertoor_params:
  image: "ethpandaops/assertoor:latest"
  run_stability_check: true
  run_block_proposal_check: true
  run_transaction_test: true
  run_blob_transaction_test: true
  tests:
    - file: "/tests/prysm-finalization.yaml"
    - file: "/tests/prysm-validators.yaml"
    - file: "/tests/prysm-deposits.yaml"
    - file: "/tests/prysm-exits.yaml"
    - file: "/tests/prysm-withdrawals.yaml"

wait_for_finalization: true
global_log_level: "info"
```

### Approach 2: Go Integration with Kurtosis SDK

For deeper integration, use Kurtosis Go SDK directly in the test code.

#### Implementation Example

```go
// kurtosis_e2e_test.go
package endtoend

import (
    "context"
    "testing"
    "time"
    
    "github.com/kurtosis-tech/kurtosis/api/golang/core/lib/enclaves"
    "github.com/kurtosis-tech/kurtosis/api/golang/core/lib/services"
    "github.com/kurtosis-tech/kurtosis/api/golang/engine/lib/kurtosis_context"
)

func TestEndToEndWithKurtosis(t *testing.T) {
    ctx := context.Background()
    
    // Create Kurtosis context
    kurtosisCtx, err := kurtosis_context.NewKurtosisContextFromLocalEngine()
    require.NoError(t, err)
    defer kurtosisCtx.Close()
    
    // Create enclave
    enclaveName := fmt.Sprintf("prysm-e2e-%d", time.Now().Unix())
    enclaveCtx, err := kurtosisCtx.CreateEnclave(ctx, enclaveName)
    require.NoError(t, err)
    defer kurtosisCtx.DestroyEnclave(ctx, enclaveName)
    
    // Load and run ethereum-package
    ethereumPackage := "github.com/ethpandaops/ethereum-package"
    runConfig := getKurtosisConfig() // Returns config as JSON string
    
    starlarkRun, err := enclaveCtx.RunStarlarkRemotePackage(
        ctx,
        ethereumPackage,
        &kurtosis_context.StarlarkRunOptions{
            SerializedParams: runConfig,
        },
    )
    require.NoError(t, err)
    
    // Get service URLs
    services, err := enclaveCtx.GetServices(ctx)
    require.NoError(t, err)
    
    // Run evaluators
    runEvaluators(t, ctx, services)
}

func getKurtosisConfig() string {
    config := map[string]interface{}{
        "participants": []map[string]interface{}{
            {
                "el_type": "geth",
                "cl_type": "prysm",
                "count":   2,
            },
        },
        "network_params": map[string]interface{}{
            "seconds_per_slot": 12,
            "epochs_to_run":    16,
        },
        "additional_services": []string{
            "assertoor",
            "tx_fuzz",
        },
    }
    
    jsonBytes, _ := json.Marshal(config)
    return string(jsonBytes)
}
```

### Approach 3: Hybrid Integration

Combine both approaches for maximum flexibility:

1. **Bazel manages the test lifecycle**
2. **Go code interfaces with Kurtosis SDK**
3. **Assertoor handles test evaluation**

```go
// BUILD.bazel
go_test(
    name = "go_default_test",
    srcs = ["kurtosis_integration_test.go"],
    deps = [
        "@com_github_kurtosis_tech_kurtosis//api/golang/core/lib/enclaves",
        "@com_github_kurtosis_tech_kurtosis//api/golang/engine/lib/kurtosis_context",
        "//testing/endtoend/evaluators:go_default_library",
    ],
    tags = ["e2e", "integration"],
)
```

## Evaluator Migration Strategy

### Phase 1: Core Evaluators to Assertoor

Map Prysm evaluators to Assertoor tests:

| Prysm Evaluator | Assertoor Test | Status |
|-----------------|----------------|--------|
| FinalizationOccurs | stability_check | ✅ Available |
| ValidatorsParticipating | participation_check | ✅ Available |
| ProcessesDepositsInBlocks | transaction_test | ✅ Available |
| ProposeVoluntaryExit | lifecycle_test | ✅ Available |
| ValidatorsHaveWithdrawn | lifecycle_test | ✅ Available |
| AllNodesHaveSameHead | stability_check | ✅ Available |
| PeersConnect | Custom test needed | ❌ Gap |
| MetricsCheck | Custom test needed | ❌ Gap |
| ColdStateCheckpoint | Custom test needed | ❌ Gap |
| VerifyBlockGraffiti | Custom test needed | ❌ Gap |

### Phase 2: Custom Assertoor Tests

Create custom Assertoor tests for missing evaluators:

```yaml
# prysm-custom-tests.yaml
name: "Prysm Compatibility Tests"
tests:
  - name: "peers_connectivity"
    tasks:
      - name: "check_peer_count"
        type: "check_consensus_node_peer_count"
        config:
          minPeers: 1
          maxPeers: 10
          
  - name: "metrics_validation"
    tasks:
      - name: "check_metrics"
        type: "run_shell"
        config:
          shell: |
            curl -s http://beacon-1:8080/metrics | grep -q 'beacon_head_slot'
            
  - name: "graffiti_verification"
    tasks:
      - name: "check_graffiti"
        type: "check_consensus_block_proposals"
        config:
          graffitiPattern: "prysm-.*"
```

## Configuration Mapping

### Network Parameters

| Prysm E2E Config | Kurtosis Config | Notes |
|------------------|-----------------|-------|
| `EpochsToRun: 16` | `wait_for_finalization: true` + timeout | Use Assertoor for epoch control |
| `TestSync: true` | Additional participant with sync test | Add delayed node |
| `TestDeposits: true` | `run_transaction_test: true` | Include deposit transactions |
| `BeaconNodeCount: 2` | `participants[].count: 2` | Direct mapping |
| `ValidatorCount: 64` | `num_validator_keys_per_node: 64` | Per node configuration |
| `TestCheckpointSync: true` | Custom Assertoor test | Requires implementation |

### Service Mapping

| Prysm Component | Kurtosis Service | Configuration |
|-----------------|------------------|---------------|
| Beacon Nodes | `cl-1-prysm`, `cl-2-prysm` | Via participants |
| Validators | `vc-1-prysm`, `vc-2-prysm` | Via participants |
| ETH1 Miner | `el-1-geth` | First EL node mines |
| Transaction Generator | `tx_fuzz` or `spamoor` | Via additional_services |
| Metrics | `prometheus` + `grafana` | Via additional_services |

## CI/CD Integration

### GitHub Actions Example

```yaml
name: E2E Tests with Kurtosis

on:
  push:
    branches: [main, develop]
  pull_request:

jobs:
  e2e-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Install Kurtosis
        run: |
          echo "deb [trusted=yes] https://apt.fury.io/kurtosis-tech/ /" | \
            sudo tee /etc/apt/sources.list.d/kurtosis.list
          sudo apt update
          sudo apt install kurtosis-cli
          
      - name: Start Kurtosis Engine
        run: kurtosis engine start
        
      - name: Run E2E Tests
        run: |
          bazel test //testing/endtoend:kurtosis_e2e_test \
            --test_output=streamed \
            --test_env=CLEANUP_ON_EXIT=true
            
      - name: Collect Logs
        if: failure()
        run: |
          kurtosis enclave dump prysm-e2e-* ./logs
          
      - name: Upload Artifacts
        if: failure()
        uses: actions/upload-artifact@v3
        with:
          name: e2e-logs
          path: ./logs
```

## Implementation Roadmap

### Week 1-2: Setup and Basic Integration
- [ ] Install Kurtosis CLI in CI environment
- [ ] Create basic Kurtosis configuration matching minimal E2E
- [ ] Write shell script wrapper for Bazel integration
- [ ] Test basic network startup and finalization

### Week 3-4: Evaluator Migration
- [ ] Map existing evaluators to Assertoor tests
- [ ] Create custom Assertoor tests for gaps
- [ ] Validate test parity with existing E2E

### Week 5-6: Advanced Features
- [ ] Implement checkpoint sync testing
- [ ] Add fork transition tests
- [ ] Configure MEV testing if needed
- [ ] Add slashing and penalty tests

### Week 7-8: CI/CD and Documentation
- [ ] Integrate with existing CI pipeline
- [ ] Performance optimization
- [ ] Documentation and training
- [ ] Deprecation plan for old E2E

## Key Advantages of Kurtosis Integration

1. **Simplified Infrastructure**: No need to manage individual components
2. **Multi-client Testing**: Easy to test against other clients
3. **Reproducibility**: Deterministic test environments
4. **Modularity**: Easy to add/remove services
5. **Better Debugging**: Built-in log collection and inspection
6. **Community Alignment**: Use same tools as EthPandaOps

## Challenges and Mitigations

| Challenge | Mitigation |
|-----------|------------|
| Learning curve for Kurtosis | Provide training and documentation |
| Assertoor feature gaps | Contribute missing tests upstream |
| CI integration complexity | Start with simple wrapper, iterate |
| Performance differences | Tune Kurtosis resources and timeouts |
| Debugging failures | Use Kurtosis inspection tools |

## Recommended Next Steps

1. **Proof of Concept**: Implement Approach 1 with basic configuration
2. **Validate Parity**: Ensure core evaluators work correctly
3. **Gap Analysis**: Document all missing Assertoor tests needed
4. **Contribute Upstream**: Submit PRs for missing Assertoor features
5. **Gradual Migration**: Run both systems in parallel initially
6. **Performance Testing**: Compare execution times and resource usage
7. **Team Training**: Conduct workshops on Kurtosis usage

## Conclusion

Integrating Prysm's E2E tests with Kurtosis is feasible and offers significant benefits. The recommended approach is to start with CLI integration via Bazel (Approach 1), then gradually migrate evaluators to Assertoor tests. This allows for a smooth transition while maintaining test coverage and CI/CD compatibility.

The key to success is:
1. Maintaining test parity during migration
2. Contributing missing features to Assertoor
3. Providing clear documentation and training
4. Running both systems in parallel during transition

With proper planning and execution, this migration will result in a more maintainable, flexible, and community-aligned testing infrastructure.
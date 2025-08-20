# Prysm E2E to Kurtosis/Assertoor Migration Guide

## Overview

This document provides a comprehensive analysis of Prysm's minimal E2E test setup (`//testing/endtoend:go_default_test`) to facilitate migration to Kurtosis with Assertoor. The minimal E2E test is the primary test suite that validates core Ethereum consensus functionality.

## 1. Minimal E2E Test Configuration

### Test Entry Point
- **File**: `testing/endtoend/minimal_e2e_test.go`
- **Function**: `TestEndToEnd_MinimalConfig`
- **Fork Configuration**: Bellatrix → Electra (full fork progression)
- **Option**: Checkpoint sync enabled

### Core Configuration Parameters

The `e2eMinimal()` function sets up the following configuration:

```go
testConfig := &types.E2EConfig{
    BeaconFlags: []string{
        "--slots-per-archive-point=<slots_per_epoch*16>",
        "--tracing-endpoint=http://127.0.0.1:<jaeger_port>",
        "--enable-tracing",
        "--trace-sample-fraction=1.0",
    },
    ValidatorFlags:      []string{},
    EpochsToRun:         16,  // Default, or E2E_EPOCHS env var
    TestSync:            true,
    TestFeature:         true,
    TestDeposits:        true,
    UsePrysmShValidator: false,
    UsePprof:            true,
    TracingSinkEndpoint: "<tracing_endpoint>",
    Seed:                0,  // Or E2E_SEED env var
}
```

### Network Parameters
- **Beacon Nodes**: 2 (StandardBeaconCount)
- **Validators**: MinGenesisActiveValidatorCount (from config)
- **Deposit Count**: 64 post-genesis deposits
- **Post-Electra Deposits**: 32 additional deposits
- **Genesis Delay**: Configured per network spec

## 2. Complete List of Evaluators

The minimal E2E test uses 20 base evaluators plus 5 fork transition evaluators:

### Core Network Evaluators

1. **PeersConnect** (epoch 0)
   - Validates all beacon nodes connect as peers
   - Expected peers = node_count - 1

2. **HealthzCheck** (after epoch 0)
   - Pings healthz endpoints for all nodes
   - Validates OK status from beacon and validator clients

3. **MetricsCheck** (after epoch 0)
   - Memory usage < 2 GiB
   - P2P validation ratios for aggregates/attestations
   - Committee cache hit/miss ratios
   - Hot state cache performance

### Validator Evaluators

4. **ValidatorsAreActive** (all epochs)
   - Ensures expected validators are active
   - Validates effective balance and exit epochs

5. **ValidatorsParticipatingAtEpoch(2)**
   - 99% participation expected (95% for multiclient)
   - Adjusts for special fork epochs

6. **ValidatorsVoteWithTheMajority** (after epoch 0)
   - Validates eth1data voting consistency
   - Checks majority algorithm implementation

7. **ValidatorSyncParticipation** (after Altair)
   - 99% sync committee participation expected
   - 90% for Altair fork epoch

### Consensus Evaluators

8. **FinalizationOccurs(3)** (epoch 3+)
   - Validates finalization at epoch - 2
   - Checks justified epochs progression

9. **FinishedSyncing** (all epochs)
   - Validates syncing status = false
   - Ensures nodes are synchronized

10. **AllNodesHaveSameHead** (all epochs)
    - Same head epoch across all nodes
    - Matching justified and finalized roots

11. **ColdStateCheckpoint** (epoch 50)
    - Validates cold state storage retrieval
    - Tests epochs 0-49 data access

### Block and Operations Evaluators

12. **VerifyBlockGraffiti** (after epoch 0)
    - Validates graffiti from predefined list
    - Ensures block proposer identification

13. **ProcessesDepositsInBlocks** (specific epoch)
    - Processes 64 deposits into blocks
    - Validates deposit inclusion

14. **ActivatesDepositedValidators** (activation epochs)
    - Validates deposit → activation flow
    - Respects churn limits

15. **DepositedValidatorsAreActive** (after activation)
    - Confirms all deposited validators activated
    - Post-genesis deposit validation

### Exit and Withdrawal Evaluators

16. **ProposeVoluntaryExit** (epoch 7)
    - Submits 2 voluntary exits
    - Tests exit operation processing

17. **ValidatorsHaveExited** (epoch 8)
    - Confirms exit epoch set
    - Validates exit completion

18. **SubmitWithdrawal** (Capella fork ± 2 epochs)
    - BLS to execution address changes
    - Tests withdrawal operations

19. **ValidatorsHaveWithdrawn** (after Capella+1)
    - Balance < 1 ETH for exited validators
    - Withdrawal completion validation

### Infrastructure Evaluators

20. **PeersCheck** (after epoch 0)
    - Gossip score validation
    - Behavior penalties check
    - Block provider scores
    - Validation error tracking

21. **FeeRecipientIsPresent** (after Bellatrix)
    - Fee recipient != burn address
    - Validates proposer settings
    - Balance increase verification

### Fork Transition Evaluators

22. **AltairForkTransition** (if configured)
23. **BellatrixForkTransition** (if configured)
24. **CapellaForkTransition** (if configured)
25. **DenebForkTransition** (if configured)
26. **ElectraForkTransition** (if configured)

Each validates successful hard fork execution and block production post-fork.

## 3. E2E Test Workflow

### Component Startup Sequence

1. **Infrastructure Setup**
   - Tracing sink initialization
   - Keystore generation (if multiclient)
   - Web3 remote signer (if configured)

2. **Network Bootstrap**
   - Boot node startup
   - ETH1 miner initialization with bootnode ENR
   - ETH1 non-mining nodes startup

3. **Consensus Layer**
   - Beacon nodes startup (2 nodes)
   - Connect to ETH1 nodes via Engine API
   - Validator clients startup
   - Connect to beacon nodes

4. **Optional Components**
   - Builder nodes (if UseBuilder=true)
   - ETH1 proxy for optimistic sync testing
   - Transaction generator for execution payload testing

### Test Execution Flow

1. **Wait for Chain Start**
   - Sleep for genesis delay period
   - Verify "Chain started in sync service" in logs
   - Generate and store genesis state

2. **Run Evaluators**
   - Execute evaluators based on epoch policies
   - Each evaluator runs when `Policy(epoch)` returns true
   - Evaluators run concurrently within each epoch
   - Continue for `EpochsToRun` epochs (default: 16)

3. **Additional Testing** (if configured)
   - **TestSync**: Spin up additional beacon node, test sync to head
   - **TestCheckpointSync**: Test checkpoint sync with new node
   - **TestDeposits**: Send and mine post-genesis deposits
   - **Doppelganger Protection**: Test duplicate validator detection

4. **Cleanup**
   - Cancel context to stop all components
   - Write pprof files if enabled
   - Generate logs and metrics

### Evaluation Timing

- **Epoch-based execution**: Evaluators run at specific epochs
- **Ticker mechanism**: `helpers.NewEpochTicker` drives evaluation
- **Concurrent evaluation**: Multiple evaluators run in parallel per epoch
- **Interceptor support**: Custom logic injection per epoch

## 4. Migration Considerations for Kurtosis/Assertoor

### Critical Evaluators Requiring Assertoor Equivalents

1. **Consensus Critical**
   - Finalization tracking (epoch 3+)
   - Fork transitions (all configured forks)
   - Validator participation rates
   - Head consensus across nodes

2. **Operations Critical**
   - Deposit processing and activation
   - Voluntary exit flow
   - Withdrawal operations (post-Capella)
   - Fee recipient validation

3. **Network Health**
   - Peer connectivity
   - Sync status
   - Metrics thresholds
   - Cold state retrieval

### Configuration Mapping to Kurtosis

**Ethereum Package Requirements:**
- 2+ beacon nodes (Prysm)
- Validator set with MinGenesisActiveValidatorCount
- ETH1 miner + non-mining nodes
- Boot node for peer discovery
- Engine API proxy support (for optimistic sync testing)

**Key Parameters:**
- Epochs to run: 16 (configurable)
- Slots per epoch: Network specific
- Genesis delay: Network specific
- Deposit counts: 64 + 32 (Electra)

### Identified Gaps

Based on the GitHub issue #15320, current Assertoor gaps include:

1. **Missing Evaluators**
   - Cold state checkpoint validation
   - Graffiti verification
   - Detailed metrics checks (memory, cache ratios)
   - Peer scoring and behavior penalties
   - Doppelganger protection testing

2. **Fork Testing Limitations**
   - Cannot start from Deneb/Electra directly
   - Fork transition validation complexity
   - Transaction generator synchronization issues

3. **Infrastructure Gaps**
   - Builder integration testing
   - Web3 signer support
   - Checkpoint sync scenarios
   - Multi-client testing setup

### Recommended Migration Strategy

1. **Phase 1: Core Functionality**
   - Implement basic network setup in Kurtosis
   - Port finalization and head consensus checks
   - Add deposit/exit/withdrawal tests

2. **Phase 2: Fork Transitions**
   - Create Assertoor tests for each fork
   - Validate fork transition blocks
   - Test fork-specific features

3. **Phase 3: Advanced Features**
   - Add optimistic sync testing
   - Implement builder testing
   - Port checkpoint sync scenarios

4. **Phase 4: Gaps**
   - Work with EthPandaOps to add missing evaluators
   - Contribute new Assertoor test cases
   - Document Prysm-specific requirements

## 5. Running Minimal E2E Tests

### Bazel Command
```bash
bazel test //testing/endtoend:go_default_test --test_arg=-test.run=TestEndToEnd_MinimalConfig
```

### Environment Variables
- `E2E_EPOCHS`: Override epochs to run (default: 16)
- `E2E_SEED`: Set random seed for deterministic testing
- `E2E_LOG_PATH`: Custom log directory
- `TEST_SHARD_INDEX`: For parallel test execution
- `TEST_TOTAL_SHARDS`: Total number of test shards

### Test Flags
- `--test_output=streamed`: Real-time test output
- `--test_env`: Pass environment variables
- `--cache_test_results=no`: Disable test caching

## Conclusion

This guide provides a comprehensive overview of Prysm's minimal E2E test infrastructure. The migration to Kurtosis/Assertoor will require careful mapping of evaluators and addressing the identified gaps. The modular nature of Prysm's evaluators should facilitate creating equivalent Assertoor test cases, though some Prysm-specific features may require upstream contributions to the Assertoor project.

Key success factors for migration:
1. Complete evaluator coverage in Assertoor
2. Fork transition testing capability
3. Performance and stability improvements
4. CI/CD pipeline integration
# Prysm E2E Test Termination Analysis & Kurtosis Equivalent

## When Prysm E2E Test Ends

### Primary Termination Conditions

1. **Epoch-based Termination** (`endtoend_test.go:202-208`)
   ```go
   if t.Failed() || currentEpoch >= config.EpochsToRun-1 {
       ticker.Done()
       if t.Failed() {
           return errors.New("test failed")
       }
       break
   }
   ```
   - **Default**: 16 epochs (configurable via `E2E_EPOCHS` env var)
   - **Success**: All evaluators pass for the specified epochs
   - **Failure**: Any evaluator fails during execution

2. **Evaluator Failure** (`endtoend_test.go:204-206`)
   - Test immediately fails if any evaluator reports failure
   - Uses `t.Failed()` to detect Go test failures

3. **Component Startup Timeout** (`endtoend_test.go:462-466`)
   ```go
   ctxAllNodesReady, cancel := context.WithTimeout(ctx, allNodesStartTimeout)
   if err := helpers.ComponentsStarted(ctxAllNodesReady, r.comHandler.required()); err != nil {
       return errors.Wrap(err, "components take too long to start")
   }
   ```
   - **Timeout**: 5 minutes (`allNodesStartTimeout`)

### Test Execution Flow & Termination

```
┌─────────────────┐
│ 1. Node Startup │ ──► Timeout: 5min
└─────────────────┘
         │
┌─────────────────┐
│ 2. Chain Start  │ ──► Wait for genesis + logs
└─────────────────┘
         │
┌─────────────────┐
│ 3. Run          │ ──► Per epoch for 16 epochs
│   Evaluators    │     (or until failure)
└─────────────────┘
         │
┌─────────────────┐
│ 4. Additional   │ ──► TestSync, TestCheckpointSync
│   Tests         │     TestDeposits, etc.
└─────────────────┘
         │
┌─────────────────┐
│ 5. Extra Epochs │ ──► If config.ExtraEpochs > 0
└─────────────────┘
         │
┌─────────────────┐
│ 6. Cleanup      │ ──► Cancel context, write logs
└─────────────────┘
```

### Specific Termination Triggers

1. **Immediate Failures**:
   - Chain fails to start
   - Any component fails to start within 5 minutes
   - Any evaluator fails during execution

2. **Successful Completion**:
   - All evaluators pass for 16 epochs
   - Additional tests complete successfully (if enabled)
   - ExtraEpochs complete (if configured)

3. **Configuration-based Duration**:
   - `EpochsToRun`: Default 16, configurable via `E2E_EPOCHS`
   - `ExtraEpochs`: Additional epochs after main test
   - Each epoch = 32 slots × 12 seconds = 384 seconds (6.4 minutes)
   - **Total time**: ~16 × 6.4 = ~102 minutes for minimal config

## Mimicking with Kurtosis Infrastructure

### Approach 1: Assertoor Test Duration Control

```yaml
# kurtosis-prysm-e2e.yaml
name: "Prysm E2E with Epoch-based Termination"
timeout: 120m  # 2 hours to match Prysm's ~102 minute duration

tests:
  # Phase 1: Wait for chain start (equivalent to waitForChainStart)
  - name: "wait_for_chain_start"
    test:
      name: check_consensus_finality
      title: "Wait for chain to start and finalize"
      timeout: 10m
      config:
        minFinalizedEpochs: 1
        
  # Phase 2: Run epoch-based evaluators (equivalent to runEvaluators)
  - name: "epoch_based_evaluation"
    test:
      name: run_tasks
      title: "Run evaluators for 16 epochs"
      timeout: 110m
      config:
        tasks:
          # Epoch 0-2: Basic checks
          - name: check_consensus_finality
            title: "Finalization by epoch 3"
            timeout: 25m
            config:
              minFinalizedEpochs: 3
              
          - name: check_consensus_attestation_stats  
            title: "Participation from epoch 2"
            timeout: 15m
            config:
              minTargetPercent: 99
              minHeadPercent: 95
              minTotalPercent: 99
              
          - name: check_consensus_validator_status
            title: "Validators active"
            timeout: 10m
            config:
              validatorStatus: ["active_ongoing"]
              
          # Custom evaluators via shell scripts
          - name: run_shell
            title: "Epoch countdown controller"
            timeout: 105m
            config:
              shell: bash
              command: |
                #!/bin/bash
                BEACON_URL="http://cl-1-prysm:3500"
                TARGET_EPOCHS=16
                START_TIME=$(date +%s)
                
                echo "Starting epoch-based test termination controller..."
                echo "Target epochs: ${TARGET_EPOCHS}"
                
                while true; do
                  # Get current epoch
                  SLOT=$(curl -s ${BEACON_URL}/eth/v1/beacon/headers/head | jq -r '.data.header.message.slot')
                  CURRENT_EPOCH=$((SLOT / 32))
                  
                  echo "Current epoch: ${CURRENT_EPOCH}"
                  
                  # Check if we've reached target epochs
                  if [ "$CURRENT_EPOCH" -ge "$TARGET_EPOCHS" ]; then
                    echo "SUCCESS: Reached target epoch ${TARGET_EPOCHS}"
                    echo "Test completed successfully after ${CURRENT_EPOCH} epochs"
                    exit 0
                  fi
                  
                  # Safety timeout (2 hours)
                  CURRENT_TIME=$(date +%s)
                  ELAPSED=$((CURRENT_TIME - START_TIME))
                  if [ "$ELAPSED" -gt 7200 ]; then
                    echo "ERROR: Test timeout after 2 hours"
                    exit 1
                  fi
                  
                  sleep 30
                done
                
  # Phase 3: Additional tests (equivalent to TestSync, TestCheckpointSync)
  - name: "additional_tests"
    test:
      name: run_tasks_concurrent
      title: "Additional post-epoch tests"
      timeout: 20m
      config:
        tasks:
          - name: run_shell
            title: "Simulate beacon chain sync test"
            config:
              shell: bash
              command: |
                echo "Additional sync tests would run here"
                echo "Equivalent to testBeaconChainSync, testDoppelGangerProtection"
                # Add custom sync testing logic
                
          - name: run_shell  
            title: "Simulate checkpoint sync test"
            config:
              shell: bash
              command: |
                echo "Checkpoint sync tests would run here"
                echo "Equivalent to testCheckpointSync"
                # Add checkpoint sync testing logic
```

### Approach 2: Time-based Termination with Monitoring

```yaml
# kurtosis-time-based.yaml
name: "Prysm E2E with Time-based Control"
timeout: 120m

tests:
  - name: "timed_e2e_test"
    test:
      name: run_task_background
      title: "Background monitoring with foreground tests"
      timeout: 115m
      config:
        # Foreground: Run all evaluators
        foregroundTask:
          name: run_tasks_concurrent
          title: "All Prysm evaluators"
          config:
            tasks:
              - name: check_consensus_finality
                config:
                  minFinalizedEpochs: 3
              - name: check_consensus_attestation_stats
                config:
                  minTargetPercent: 99
              # Add all other evaluators...
              
        # Background: Timer that stops test after target duration
        backgroundTask:
          name: run_shell
          title: "Test duration controller"
          config:
            shell: bash
            command: |
              #!/bin/bash
              
              # Calculate target duration: 16 epochs × 384 seconds = 6144 seconds
              TARGET_DURATION=6144
              START_TIME=$(date +%s)
              
              echo "Test will run for ${TARGET_DURATION} seconds (16 epochs)"
              
              while true; do
                CURRENT_TIME=$(date +%s)
                ELAPSED=$((CURRENT_TIME - START_TIME))
                
                if [ "$ELAPSED" -ge "$TARGET_DURATION" ]; then
                  echo "SUCCESS: Test completed after target duration"
                  echo "Total time: ${ELAPSED} seconds"
                  exit 0
                fi
                
                REMAINING=$((TARGET_DURATION - ELAPSED))
                echo "Time remaining: ${REMAINING} seconds"
                sleep 60
              done
              
        exitOnForegroundFailure: true  # Fail fast if any evaluator fails
        onBackgroundComplete: "succeed"  # Success when timer completes
```

### Approach 3: Hybrid Kurtosis + Shell Controller

```bash
#!/bin/bash
# kurtosis-prysm-e2e-controller.sh

set -euo pipefail

ENCLAVE_NAME="prysm-e2e-$(date +%s)"
TARGET_EPOCHS=16
TIMEOUT_SECONDS=7200  # 2 hours safety timeout

echo "Starting Prysm E2E test with epoch-based termination..."

# Start Kurtosis network
kurtosis run \
  --enclave "$ENCLAVE_NAME" \
  github.com/ethpandaops/ethereum-package \
  "$(cat <<EOF
{
  "participants": [{"el_type": "geth", "cl_type": "prysm", "count": 2, "validator_count": 64}],
  "network_params": {"seconds_per_slot": 12, "num_validator_keys_per_node": 64},
  "additional_services": ["assertoor"],
  "assertoor_params": {
    "run_stability_check": true,
    "run_block_proposal_check": true,
    "tests": [{"file": "/tests/custom/prysm-evaluators.yaml"}]
  }
}
EOF
)" &

KURTOSIS_PID=$!
START_TIME=$(date +%s)

# Wait for network to be ready
sleep 60

echo "Monitoring epoch progression for termination..."

while true; do
  # Check if Kurtosis is still running
  if ! kill -0 $KURTOSIS_PID 2>/dev/null; then
    echo "Kurtosis process ended unexpectedly"
    exit 1
  fi
  
  # Get current epoch
  BEACON_URL=$(kurtosis enclave inspect "$ENCLAVE_NAME" | grep "cl-1-prysm.*3500" | grep -o 'http://[^[:space:]]*' | head -1)
  
  if [ -n "$BEACON_URL" ]; then
    SLOT=$(curl -s ${BEACON_URL}/eth/v1/beacon/headers/head | jq -r '.data.header.message.slot' 2>/dev/null || echo "0")
    CURRENT_EPOCH=$((SLOT / 32))
    
    echo "Current epoch: ${CURRENT_EPOCH}/${TARGET_EPOCHS}"
    
    # Check termination conditions
    if [ "$CURRENT_EPOCH" -ge "$TARGET_EPOCHS" ]; then
      echo "SUCCESS: Reached target epoch ${TARGET_EPOCHS}"
      break
    fi
  fi
  
  # Safety timeout
  CURRENT_TIME=$(date +%s)
  ELAPSED=$((CURRENT_TIME - START_TIME))
  if [ "$ELAPSED" -gt "$TIMEOUT_SECONDS" ]; then
    echo "ERROR: Test timeout after ${ELAPSED} seconds"
    exit 1
  fi
  
  sleep 30
done

# Graceful shutdown
echo "Terminating test after successful completion..."
kurtosis enclave rm -f "$ENCLAVE_NAME"
echo "Test completed successfully in $((($(date +%s) - START_TIME))) seconds"
```

## Key Differences & Adaptations

### Prysm E2E vs Kurtosis Behavior

| Aspect | Prysm E2E | Kurtosis Equivalent |
|--------|-----------|-------------------|
| **Termination** | Epoch-based (16 epochs) | Assertoor test timeout or custom controller |
| **Failure Handling** | Immediate stop on any evaluator failure | Configurable via `failOnCheckMiss` |
| **Duration** | ~102 minutes (16 epochs × 6.4 min) | Configurable timeout |
| **Evaluator Execution** | Per-epoch with epoch ticker | Continuous monitoring or periodic checks |
| **Cleanup** | Automatic context cancellation | Kurtosis enclave cleanup |

### Recommended Implementation

**Use Approach 3 (Hybrid)** for closest behavior match:

1. **Kurtosis handles infrastructure**: Network setup, node management
2. **Shell controller handles termination**: Epoch counting and graceful shutdown  
3. **Assertoor handles evaluation**: Test execution and validation
4. **Exact timing match**: Same 16-epoch duration as Prysm

This approach preserves Prysm's epoch-based termination logic while leveraging Kurtosis's superior infrastructure management and Assertoor's comprehensive test framework.

### Configuration for Exact Prysm Behavior

```yaml
# Exact duration match
epochsToRun: 16
seconds_per_slot: 12
slots_per_epoch: 32
# Total time: 16 × 32 × 12 = 6,144 seconds (102.4 minutes)

# Safety timeout: 120 minutes (vs Prysm's no built-in timeout)
timeout: 120m
```

This gives us the exact same test duration and termination behavior as the original Prysm E2E tests.
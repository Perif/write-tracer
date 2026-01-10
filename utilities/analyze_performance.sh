#!/bin/bash
set -e

# Configuration
TRACER_BIN="../write-tracer"
TEST_FILE="perf_test.tmp"
DD_COUNT=200000
DD_BS="4k"
ITERATIONS=3
SIM_EPOCHS=5
SIM_CHECKPOINT_MB=50


if [ ! -f "$TRACER_BIN" ]; then
    echo "Error: Tracer binary not found at $TRACER_BIN"
    echo "Please build it first: make build"
    exit 1
fi

cleanup() {
    rm -f "$TEST_FILE"
}
trap cleanup EXIT

echo "============================================="
echo " Write Tracer Performance Benchmark"
echo " Workload: dd via writes of $DD_BS x $DD_COUNT"
echo "============================================="

run_dd_baseline() {
    echo "Running Baseline (No Tracer)..." >&2
    local total_mb=$(echo "$DD_COUNT * 4 / 1024" | bc) 
    
    # We use 'time' to measure real duration, and handle dd's output
    # dd prints stats to stderr. unique throughput format varies by version, so we calculate from time.
    
    start_time=$(date +%s.%N)
    dd if=/dev/zero of="$TEST_FILE" bs="$DD_BS" count="$DD_COUNT" status=none
    end_time=$(date +%s.%N)
    
    duration=$(echo "$end_time - $start_time" | bc)
    throughput=$(echo "$total_mb / $duration" | bc)
    echo "$throughput"
}

run_dd_traced() {
    local bg_tracer=$1
    local mode_name=$2
    
    echo "Running with Tracer ($mode_name)..." >&2
    
    # Start a dummy parent process that we can attach to.
    # We will run dd as a child of this shell, so tracking the shell PID should work 
    # if trace-children is implicit (which it is in write-tracer).
    
    # Actually, the easier way is to start a sleep process, attach to it, 
    # and then spawn dd from that same shell context? 
    # No, write-tracer attaches to a PID. If we attach to the current shell $$?
    
    # Strategy: Start a subshell that waits for a signal to run dd.
    # But for simplicity: Start the tracer on THIS shell's PID in background?
    # Risky if it traces the script commands itself.
    
    # Better Strategy:
    # 1. Start a sleep process in background necessary to keep PID alive? 
    #    No, we want to trace the dd process.
    #    But we have to start tracer BEFORE dd.
    #    And we don't know dd's PID before it starts.
    #    
    #    Solution: Use 'utilities/echopid' or similar approach?
    #    Or: 
    #    Start a shell, get its PID, attach tracer to shell, then send command to shell to run dd.
    
    # Let's use a background subshell
    (
        local subshell_pid=$BASHPID
        # Signal parent we are ready with PID? 
        # A rough sync: write PID to file
        echo $subshell_pid > .pid_tmp
        
        # Wait for tracer to be ready (dumb sleep)
        sleep 2
        
        start_time_inner=$(date +%s.%N)
        dd if=/dev/zero of="$TEST_FILE" bs="$DD_BS" count="$DD_COUNT" status=none
        end_time_inner=$(date +%s.%N)
        
        duration_inner=$(echo "$end_time_inner - $start_time_inner" | bc)
        total_mb_inner=$(echo "$DD_COUNT * 4 / 1024" | bc)
        throughput_inner=$(echo "$total_mb_inner / $duration_inner" | bc)
        echo "$throughput_inner" > .result_tmp
        
    ) &
    bg_pid=$!
    
    # Wait for PID file
    while [ ! -f .pid_tmp ]; do sleep 0.1; done
    target_pid=$(cat .pid_tmp)
    rm .pid_tmp
    
    # Start Tracer
    if [ "$bg_tracer" == "1" ]; then
         # We need sudo for the tracer. Assuming user runs script with sudo or has NOPASSWD.
         # Run tracer in background, silenced
         if [ "$mode_name" == "Quiet" ]; then
             sudo "$TRACER_BIN" -p "$target_pid" -q > /dev/null 2>&1 &
         else
             sudo "$TRACER_BIN" -p "$target_pid" > /dev/null 2>&1 &
         fi
         tracer_pid=$!
         
         # Wait for tracer to attach (approx 1 sec usually enough)
         sleep 1
    fi
    
    wait $bg_pid
    
    if [ "$bg_tracer" == "1" ]; then
        sudo kill $tracer_pid 2>/dev/null || true
        wait $tracer_pid 2>/dev/null || true
    fi
    
    cat .result_tmp
    rm .result_tmp

}


run_simulated_training() {
    local bg_tracer=$1
    local mode_name=$2
    
    echo "Running AI Workload Simulation ($mode_name)..." >&2
    
    # Calculate dd count for checkpoint size
    # 50MB / 4KB = 12800 blocks
    local dd_count=$(echo "$SIM_CHECKPOINT_MB * 1024 * 1024 / 4096" | bc)
    
    (
        local subshell_pid=$BASHPID
        echo $subshell_pid > .pid_sim_tmp
        
        sleep 2
        
        start_time_sim=$(date +%s.%N)
        
        for i in $(seq 1 $SIM_EPOCHS); do
            # Simulate Compute (GPU)
            sleep 1
            # Simulate Checkpoint (IO Burst)
            dd if=/dev/zero of="$TEST_FILE" bs=4k count="$dd_count" status=none
        done
        
        end_time_sim=$(date +%s.%N)
        duration_sim=$(echo "$end_time_sim - $start_time_sim" | bc)
        echo "$duration_sim" > .result_sim_tmp
        
    ) &
    bg_pid=$!
    
    while [ ! -f .pid_sim_tmp ]; do sleep 0.1; done
    target_pid=$(cat .pid_sim_tmp)
    rm .pid_sim_tmp
    
    if [ "$bg_tracer" == "1" ]; then
         if [ "$mode_name" == "Quiet" ]; then
             sudo "$TRACER_BIN" -p "$target_pid" -q > /dev/null 2>&1 &
         else
             sudo "$TRACER_BIN" -p "$target_pid" > /dev/null 2>&1 &
         fi
         tracer_pid=$!
         sleep 1
    fi
    
    wait $bg_pid
    
    if [ "$bg_tracer" == "1" ]; then
        sudo kill $tracer_pid 2>/dev/null || true
        wait $tracer_pid 2>/dev/null || true
    fi
    
    cat .result_sim_tmp
    rm .result_sim_tmp
}

echo "============================================="
echo " Section 1: Stream Throughput (dd)"
echo "============================================="

# 1. Baseline
base_mbps=$(run_dd_baseline)
echo "Baseline Throughput: $base_mbps MB/s"
echo ""

# 2. Traced (Standard)
traced_mbps=$(run_dd_traced 1 "Standard")
echo "Traced (Std) Throughput: $traced_mbps MB/s"
echo ""

# 3. Traced (Quiet)
quiet_mbps=$(run_dd_traced 1 "Quiet")
echo "Traced (Quiet) Throughput: $quiet_mbps MB/s"
echo ""

echo "============================================="
echo " Section 2: AI Workload Simulation"
echo " (Compute-Bound + IO Bursts)"
echo " Epochs: $SIM_EPOCHS, Checkpoint: ${SIM_CHECKPOINT_MB}MB"
echo "============================================="

# 1. Baseline Simulation
sim_base_time=$(run_simulated_training 0 "Baseline")
echo "Baseline Duration: $sim_base_time s"
echo ""

# 2. Traced Simulation
sim_traced_time=$(run_simulated_training 1 "Standard")
echo "Traced (Std) Duration: $sim_traced_time s"
echo ""

# 3. Traced Quiet Simulation
sim_quiet_time=$(run_simulated_training 1 "Quiet")
echo "Traced (Quiet) Duration: $sim_quiet_time s"
echo ""


# Summary
echo "---------------------------------------------"
echo "Results:"
echo "Stream Throughput (Worst Case):"
echo "  Baseline:       $base_mbps MB/s"
echo "  Traced (Std):   $traced_mbps MB/s"
echo "  Traced (Quiet): $quiet_mbps MB/s"
echo ""
echo "AI Workload (Amortized):"
echo "  Baseline:       $sim_base_time s"
echo "  Traced (Std):   $sim_traced_time s"
echo "  Traced (Quiet): $sim_quiet_time s"

calc_drop() {
    base=$1
    new=$2
    # Avoid div by zero
    if [ $(echo "$base == 0" | bc) -eq 1 ]; then echo "0"; return; fi
    echo "scale=2; 100 - ($new * 100 / $base)" | bc
}

calc_increase() {
    base=$1
    new=$2
    # Avoid div by zero
    if [ $(echo "$base == 0" | bc) -eq 1 ]; then echo "0"; return; fi
    echo "scale=2; ($new - $base) * 100 / $base" | bc
}

drop_std=$(calc_drop $base_mbps $traced_mbps)
drop_quiet=$(calc_drop $base_mbps $quiet_mbps)

inc_std=$(calc_increase $sim_base_time $sim_traced_time)
inc_quiet=$(calc_increase $sim_base_time $sim_quiet_time)

echo ""
echo "Overhead Summary:"
echo "  Stream IO (Std):   $drop_std % drop"
echo "  Stream IO (Quiet): $drop_quiet % drop"
echo "  AI Workload (Std):   $inc_std % slower"
echo "  AI Workload (Quiet): $inc_quiet % slower"
echo "---------------------------------------------"

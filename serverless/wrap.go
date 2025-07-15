package serverless

import (
	"context"
	"fmt"
	"runtime"
	"time"
)

const (
	cpuCostPerSec   = 0.00001667 // ≈ $0.06 per CPU-hour
	memCostPerMBSec = 0.00000014 // ≈ $0.50 per GB-hour
	gpuCostPerSec   = 0.0005     // placeholder for GPU-second cost
)

// defaultRunner calls the function in-process.
type defaultRunner struct{}

func (r *defaultRunner) Run(ctx context.Context, fn ServerlessFunc, cfg Config, input any) (any, error) {
	return fn(ctx, input)
}

var DefaultRunner Runner = &defaultRunner{}

// Wrap returns a new ServerlessFunc that:
//  1. applies the Timeout from cfg,
//  2. records start/end time & memory stats,
//  3. delegates to DefaultRunner,
//  4. computes & prints resource usage and cost.
func Wrap(fn ServerlessFunc, cfg Config) ServerlessFunc {
	return func(ctx context.Context, input any) (any, error) {
		// apply timeout if set
		execCtx := ctx
		var cancel context.CancelFunc
		if cfg.Timeout > 0 {
			execCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
			defer cancel()
		}

		// capture start metrics
		start := time.Now()
		var memStart runtime.MemStats
		runtime.ReadMemStats(&memStart)

		// invoke
		result, err := DefaultRunner.Run(execCtx, fn, cfg, input)

		// capture end metrics
		elapsed := time.Since(start)
		var memEnd runtime.MemStats
		runtime.ReadMemStats(&memEnd)

		// compute usage
		cpuSecs := elapsed.Seconds() * float64(cfg.CPU)
		memUsedMB := float64(memEnd.Alloc-memStart.Alloc) / (1024 * 1024)
		memMBSecs := memUsedMB * elapsed.Seconds()
		gpuSecs := 0.0
		if cfg.GPU {
			gpuSecs = elapsed.Seconds()
		}

		// compute cost
		cost := cpuSecs*cpuCostPerSec + memMBSecs*memCostPerMBSec + gpuSecs*gpuCostPerSec

		// output metrics
		fmt.Printf("== Invocation Metrics ==\n")
		fmt.Printf("Duration:          %v\n", elapsed)
		fmt.Printf("CPU-seconds:       %.4f\n", cpuSecs)
		fmt.Printf("Memory (MB-sec):   %.4f\n", memMBSecs)
		if cfg.GPU {
			fmt.Printf("GPU-seconds:       %.4f\n", gpuSecs)
		}
		fmt.Printf("Estimated cost:    $%.8f\n\n", cost)

		return result, err
	}
}

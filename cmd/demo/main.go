package main

import (
	"context"
	"fmt"
	"time"

	"light_server/runner"
	"light_server/serverless"
)

func main() {
	serverless.DefaultRunner = &runner.LocalRunner{}

	busyWork := serverless.ServerlessFunc(func(ctx context.Context, input any) (any, error) {
		// simulate ~50 ms of pure CPU work
		const N = 5_000_000
		var acc uint64
		for i := 0; i < N; i++ {
			acc += uint64(i) * uint64(i)
		}
		return acc, nil
	})

	// 3) resource wrapper with a 1s timeout:
	cfg := serverless.Config{
		CPU:      1,
		MemoryMB: 64,
		GPU:      false,
		Timeout:  1 * time.Second,
		UseCache: false,
	}
	wrapped := serverless.Wrap(busyWork, cfg)

	// 4) Invoke and print execution result:
	res, err := wrapped(context.Background(), "Hello, Phase 2!")
	if err != nil {
		fmt.Println("Invocation error:", err)
		return
	}
	fmt.Println("Result:", res)
}

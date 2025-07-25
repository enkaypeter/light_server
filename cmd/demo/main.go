package main

import (
	"context"
	"fmt"
	"time"

	"light_server/runner"
	"light_server/serverless"
)

func main() {
	serverless.DefaultRunner = &runner.DockerRunner{
		Image:   "enkaypeter/mobilenet-onnx-runner:v0.1.0",
		WorkDir: "",               // defaults to current working directory
		Timeout: 60 * time.Second,
	}

	classifyFn := serverless.ServerlessFunc(func(ctx context.Context, input any) (any, error) {
		return nil, nil
	})

	// 3) resource wrapper with a 1m timeout:
	cfg := serverless.Config{
		CPU:      1,
		MemoryMB: 128, // 128 MB memory limit
		GPU:      false,
		Timeout:  60 * time.Second,
		UseCache: false,
	}

	// 4. Wrap the stub function
	wrapped := serverless.Wrap(classifyFn, cfg)

	// 5. Invoke with a sample input
	inputPath := "images/cat.jpg"
	fmt.Printf("Invoking container for input: %s\n\n", inputPath)

	result, err := wrapped(context.Background(), inputPath)
	if err != nil {
		fmt.Println("Error during invocation:", err)
		return
	}

	// 6. Print out the raw container output
	fmt.Printf("\n=== Container Output ===\n%s\n", result)
}

package main

import (
	"context"
	"fmt"

	"light_server/serverless"
)

func main() {
	// 1. Define a simple function to echo its input:
	echoFn := serverless.ServerlessFunc(func(ctx context.Context, input any) (any, error) {
		return fmt.Sprintf("Echo: %v", input), nil
	})

	// 2. Wrap it with default config (no resource hints yet):
	wrapped := serverless.Wrap(echoFn, serverless.Config{})

	// 3. Invoke:
	res, err := wrapped(context.Background(), "Hello, World!")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Println("Result:", res)
}

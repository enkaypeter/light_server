package runner

import (
	"context"

	"light_server/serverless"
)

// LocalRunner is a no-op runner for local development and testing.
type LocalRunner struct{}

// Run simply executes the function directly.
func (r *LocalRunner) Run(ctx context.Context, fn serverless.ServerlessFunc, cfg serverless.Config, input any) (any, error) {
	return fn(ctx, input)
}

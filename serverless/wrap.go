package serverless

import "context"

// defaultRunner calls the function in-process.
type defaultRunner struct{}

func (r *defaultRunner) Run(ctx context.Context, fn ServerlessFunc, cfg Config, input any) (any, error) {
	return fn(ctx, input)
}

var DefaultRunner Runner = &defaultRunner{}

// Wrap returns a new ServerlessFunc that delegates to DefaultRunner.
func Wrap(fn ServerlessFunc, cfg Config) ServerlessFunc {
	return func(ctx context.Context, input any) (any, error) {
		return DefaultRunner.Run(ctx, fn, cfg, input)
	}
}

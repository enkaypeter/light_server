package serverless

import "context"

// ServerlessFunc is the signature of any function we can wrap.
type ServerlessFunc func(ctx context.Context, input any) (any, error)

// Config holds resource hints and feature flags for a wrapped function.
type Config struct {
	CPU      int         // number of cores to request
	MemoryMB int         // memory limit in MB
	GPU      bool        // whether to request GPU
	Timeout  interface{} // placeholder: will become time.Duration
	UseCache bool        // whether to reuse warm containers
}

// Runner abstracts how a wrapped function is actually invoked.
type Runner interface {
	Run(ctx context.Context, fn ServerlessFunc, cfg Config, input any) (any, error)
}

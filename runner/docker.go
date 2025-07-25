package runner

import (
    "context"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "time"

    "light_server/serverless"
)

// DockerRunner executes ServerlessFunc invocations inside Docker containers via the CLI.
type DockerRunner struct {
    Image   string        // Docker image (e.g. "enkaypeter/onnx-demo:v0.2.1")
    WorkDir string        // Host dir to mount (defaults to cwd if empty)
    Timeout time.Duration // Max time to allow for `docker run`
}

// Run ignores fn (execution happens in the container), and uses `input` as a path relative to WorkDir.
func (r *DockerRunner) Run(ctx context.Context, _ serverless.ServerlessFunc, cfg serverless.Config, input any) (any, error) {
    // 1. Resolve workdir
    workDir := r.WorkDir
    if workDir == "" {
        wd, err := os.Getwd()
        if err != nil {
            return nil, fmt.Errorf("docker-runner: unable to get cwd: %w", err)
        }
        workDir = wd
    }

    // 2. Validate input path
    relPath, ok := input.(string)
    if !ok {
        return nil, fmt.Errorf("docker-runner: input must be string path, got %T", input)
    }
    hostPath := filepath.Join(workDir, relPath)
    if _, err := os.Stat(hostPath); err != nil {
        return nil, fmt.Errorf("docker-runner: cannot stat %q: %w", hostPath, err)
    }

    // 3. Build docker CLI args
    args := []string{
        "run", "--rm",
        "-v", fmt.Sprintf("%s:/app/%s", hostPath, relPath),
        "-w", "/app",
    }
    // apply resource flags
    if cfg.MemoryMB > 0 {
        args = append(args, "--memory", fmt.Sprintf("%dm", cfg.MemoryMB))
    }
    if cfg.GPU {
        args = append(args, "--gpus", "all")
    }
    args = append(args, r.Image, relPath)

    // 4. Prepare command with timeout
    cmdCtx := ctx
    if r.Timeout > 0 {
        var cancel context.CancelFunc
        cmdCtx, cancel = context.WithTimeout(ctx, r.Timeout)
        defer cancel()
    }
    cmd := exec.CommandContext(cmdCtx, "docker", args...)

    // 5. Run and capture output
    output, err := cmd.CombinedOutput()
    if err != nil {
        return nil, fmt.Errorf("docker-runner: error: %w\noutput: %s", err, string(output))
    }

    return string(output), nil
}

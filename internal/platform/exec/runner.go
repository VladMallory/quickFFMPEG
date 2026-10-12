package execplatform

import (
	"context"
	"io"
	"os/exec"
)

type Runner struct{}

func New() *Runner {
	return &Runner{}
}

func (r *Runner) Run(
	ctx context.Context,
	stdout, stderr io.Writer,
	name string,
	args ...string,
) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	return cmd.Run()
}

func (r *Runner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	return cmd.Output()
}

package utils

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
)

// RunCmd runs a command and preserves both execution and context errors on failure.
func RunCmd(ctx context.Context, cmd string, env []string, args ...string) error {
	command := exec.CommandContext(ctx, cmd, args...)
	if env != nil {
		command.Env = env
	}

	err := runCmd(ctx, command)
	if err != nil {
		return err
	}

	return nil
}

// RunCmdInPath runs a command in cmdPath with the same error contract as RunCmd.
func RunCmdInPath(ctx context.Context, cmdPath, cmd string, env []string, args ...string) error {
	command := exec.CommandContext(ctx, cmd, args...)
	if env != nil {
		command.Env = env
	}

	command.Dir = cmdPath

	err := runCmd(ctx, command)
	if err != nil {
		return err
	}

	return nil
}

func runCmd(ctx context.Context, command *exec.Cmd) error {
	var stdout, stderr bytes.Buffer

	command.Stdout = &stdout
	command.Stderr = &stderr

	commandStr := command.String()

	// Run reads both streams concurrently, so a command producing large output
	// on stdout and stderr at the same time cannot deadlock.
	runErr := command.Run()

	if stderr.Len() > 0 {
		slog.InfoContext(
			ctx,
			"Running command",
			slog.String("command", commandStr),
			slog.String("stderr", stderr.String()),
		)
	}

	if stdout.Len() > 0 {
		slog.DebugContext(
			ctx,
			"Running command",
			slog.String("command", commandStr),
			slog.String("stdout", stdout.String()),
		)
	}

	if runErr != nil {
		return fmt.Errorf("command failed: %w", errors.Join(runErr, ctx.Err()))
	}

	return nil
}

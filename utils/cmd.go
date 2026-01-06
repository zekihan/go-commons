package utils

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
)

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
	stderrPipe, err := command.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	stdoutPipe, err := command.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	err = command.Start()
	if err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	commandStr := command.String()

	stderr, err := io.ReadAll(stderrPipe)
	if err != nil {
		return fmt.Errorf("failed to read stderr: %w", err)
	}

	if len(stderr) > 0 {
		stderrStr := string(stderr)
		slog.InfoContext(
			ctx,
			"Running command",
			slog.String("command", commandStr),
			slog.String("stderr", stderrStr),
		)
	}

	stdout, err := io.ReadAll(stdoutPipe)
	if err != nil {
		return fmt.Errorf("failed to read stdout: %w", err)
	}

	if len(stdout) > 0 {
		stdoutStr := string(stdout)
		slog.DebugContext(
			ctx,
			"Running command",
			slog.String("command", commandStr),
			slog.String("stdout", stdoutStr),
		)
	}

	err = command.Wait()
	if err != nil {
		return fmt.Errorf("command failed: %w", err)
	}

	return nil
}

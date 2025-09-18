package infrastructure

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/tui"
)

type sequenceCommand struct {
	command string
	args    []string
}

func buildCommandSequences(command string, args []string) []sequenceCommand {
	// If using sh -c, don't parse for pipes - let the shell handle it
	if command == "sh" && len(args) > 0 && args[0] == "-c" {
		return []sequenceCommand{{command: command, args: args}}
	}

	var sequences []sequenceCommand
	current := sequenceCommand{
		command: command,
	}
	for _, arg := range args {
		if arg == "|" {
			sequences = append(sequences, current)
			current = sequenceCommand{}
		} else if current.command == "" {
			current.command = arg
		} else {
			current.args = append(current.args, arg)
		}
	}
	if current.command != "" {
		sequences = append(sequences, current)
	}
	return sequences
}

func runCommand(ctx context.Context, logger logger.Logger, message string, command string, args ...string) (string, error) {
	var err error
	var output []byte
	tui.ShowSpinner(message, func() {
		sequences := buildCommandSequences(command, args)
		var input bytes.Buffer
		for i, sequence := range sequences {
			logger.Trace("running [%d/%d]: %s %s", 1+i, len(sequences), sequence.command, strings.Join(sequence.args, " "))
			c := exec.CommandContext(ctx, sequence.command, sequence.args...)
			c.Stdin = &input
			o, oerr := c.CombinedOutput()
			if oerr != nil {
				output = o
				err = oerr
				return
			}
			input.Reset()
			input.Write(o)
		}
		output = input.Bytes()
	})
	if err != nil {
		logger.Trace("ran: %s, errored: %s", command, strings.TrimSpace(string(output)), err)

		// Handle AWS "already exists" errors as success since resource is in desired state
		outputStr := strings.TrimSpace(string(output))
		if strings.Contains(outputStr, "EntityAlreadyExists") ||
			strings.Contains(outputStr, "AlreadyExists") ||
			strings.Contains(outputStr, "already exists") {
			logger.Trace("AWS resource already exists, treating as success")
			return outputStr, nil
		}

		// Include command output in the error for better debugging
		return outputStr, fmt.Errorf("command failed: %w\nOutput: %s", err, outputStr)
	}
	logger.Trace("ran: %s %s", command, strings.TrimSpace(string(output)))
	return string(output), nil
}

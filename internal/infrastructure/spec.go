package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/agentuity/go-common/logger"
	cstr "github.com/agentuity/go-common/string"
)

var ErrInvalidMatch = errors.New("validation failed")

type Validation string

func (v *Validation) Matches(ctx ExecutionContext, s string) error {
	if v == nil || *v == "" {
		return nil
	}
	vals, err := ctx.Interpolate(string(*v))
	if err != nil {
		return err
	}
	r, err := regexp.Compile(vals[0])
	if err != nil {
		return err
	}
	if r.MatchString(s) {
		return nil
	}
	return errors.Join(ErrInvalidMatch, fmt.Errorf("expected output to match %s. (%s)", *v, s))
}

type ExecutionCommand struct {
	Message   string     `json:"message"`
	Command   string     `json:"command"`
	Arguments []string   `json:"arguments"`
	Validate  Validation `json:"validate,omitempty"`
	Success   string     `json:"success,omitempty"`
}

func (c *ExecutionCommand) Run(ctx ExecutionContext) error {
	args, err := ctx.Interpolate(c.Arguments...)
	if err != nil {
		return err
	}
	output, err := runCommand(ctx.Context, ctx.Logger, c.Message, c.Command, args...)
	if err != nil {
		return err
	}
	out := strings.TrimSpace(string(output))
	return c.Validate.Matches(ctx, out)
}

type ExecutionSpec struct {
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Execute     ExecutionCommand  `json:"execute"`
	SkipIf      *ExecutionCommand `json:"skip_if,omitempty"`
}

func (s *ExecutionSpec) Run(ctx ExecutionContext) error {
	args, err := ctx.Interpolate(s.Execute.Arguments...)
	if err != nil {
		return err
	}
	return execAction(
		ctx.Context,
		ctx.Runnable,
		s.Title,
		s.Description,
		s.Execute.Command,
		args,
		func(_ctx context.Context, cmd string, args []string) error {
			return s.Execute.Run(ctx)
		},
		s.Execute.Success,
		func(_ctx context.Context) (bool, error) {
			if s.SkipIf != nil {
				if err := s.SkipIf.Run(ctx); err != nil {
					// If skip_if command fails (e.g., resource doesn't exist), don't skip
					// Only propagate validation errors, not command execution errors
					if errors.Is(err, ErrInvalidMatch) {
						return false, nil
					}
					// For other errors (like AWS NoSuchEntity), treat as "don't skip"
					return false, nil
				}
				return true, nil
			}
			return false, nil
		},
	)
}

type ExecutionContext struct {
	Context     context.Context
	Logger      logger.Logger
	Environment map[string]any
	Runnable    bool
}

func (c *ExecutionContext) Interpolate(args ...string) ([]string, error) {
	var newargs []string
	for _, arg := range args {
		val, err := cstr.Interpolate(arg, func(key string) (any, bool) {
			if v, ok := c.Environment[key]; ok {
				return v, true
			}
			return nil, false
		})
		if err != nil {
			return nil, err
		}
		newargs = append(newargs, val)
	}
	return newargs, nil
}

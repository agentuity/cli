package bundler

import (
	"context"
	"io"

	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/project"
)

// BundleContext holds the context for bundling operations
type BundleContext struct {
	Context        context.Context
	Logger         logger.Logger
	Project        *project.Project
	ProjectDir     string
	Production     bool
	Install        bool
	CI             bool
	DevMode        bool
	Writer         io.Writer
	PromptsEvalsFF bool
}

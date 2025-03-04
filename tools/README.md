# Error Code System

This directory contains tools for managing error codes in the CLI application.

## Error Code Generator

The `generate_error_codes.go` script generates Go code for error types based on definitions in the `error_codes.yaml` file at the root of the project.

### How it works

1. Error codes are defined in `error_codes.yaml` in the following format:
   ```yaml
   errors:
     - code: CLI-0001
       message: Failed to delete agents
     - code: CLI-0002
       message: Failed to create project
   ```

2. The code can be generated in two ways:
   - Using `go generate ./...` or `make go-generate` (recommended)
   - Using the legacy method: `go run tools/generate_error_codes.go` or `make generate`

Both methods will:
   - Read the YAML file
   - Generate appropriate variable names based on the error messages
   - Create the `internal/errsystem/errorcodes.go` file with the defined error types

### Adding new error codes

To add a new error code:

1. Edit `error_codes.yaml` and add a new entry with a unique code and descriptive message
2. Run `go generate ./...` or `make go-generate` to update the Go code
3. Use the generated error type in your code

### Naming convention

The generator creates variable names by:
- Removing common words like "failed", "unable", "to", etc.
- Converting the remaining words to CamelCase
- Prefixing with "Err"

For example:
- "Failed to delete agents" becomes `ErrDeleteAgents`
- "Unable to authenticate user" becomes `ErrAuthenticateUser`

## Usage in code

Use the generated error types with the `errsystem.New` function:

```go
import "your-project/internal/errsystem"

func someFunction() error {
    // ...
    if err != nil {
        return errsystem.New(errsystem.ErrDeleteAgents, err)
    }
    // ...
}
``` 
// Code generated by tools/generate_error_codes.go; DO NOT EDIT.
package errsystem

var (
	ErrDeleteAgents = errorType{
		Code:    "CLI-0001",
		Message: "Failed to delete agents",
	}
	ErrCreateProject = errorType{
		Code:    "CLI-0002",
		Message: "Failed to create project",
	}
	ErrAuthenticateUser = errorType{
		Code:    "CLI-0003",
		Message: "Unable to authenticate user",
	}
	ErrEnvironmentVariablesNotSet = errorType{
		Code:    "CLI-0004",
		Message: "Environment variables not set",
	}
	ErrApiRequest = errorType{
		Code:    "CLI-0005",
		Message: "API request failed",
	}
	ErrInvalidConfiguration = errorType{
		Code:    "CLI-0006",
		Message: "Invalid configuration",
	}
	ErrSaveProject = errorType{
		Code:    "CLI-0007",
		Message: "Failed to save project",
	}
	ErrDeployProject = errorType{
		Code:    "CLI-0008",
		Message: "Failed to deploy project",
	}
	ErrUploadProject = errorType{
		Code:    "CLI-0009",
		Message: "Failed to upload project",
	}
	ErrParseEnvironmentFile = errorType{
		Code:    "CLI-0010",
		Message: "Failed to parse environment file",
	}
	ErrInvalidCommandFlag = errorType{
		Code:    "CLI-0011",
		Message: "Invalid command flag error",
	}
	ErrListFilesAndDirectories = errorType{
		Code:    "CLI-0012",
		Message: "Failed to list files and directories",
	}
	ErrWriteConfigurationFile = errorType{
		Code:    "CLI-0013",
		Message: "Failed to write configuration file",
	}
	ErrReadConfigurationFile = errorType{
		Code:    "CLI-0014",
		Message: "Failed to read configuration file",
	}
	ErrCreateDirectory = errorType{
		Code:    "CLI-0015",
		Message: "Failed to create directory",
	}
	ErrCreateTemporaryFile = errorType{
		Code:    "CLI-0016",
		Message: "Failed to create temporary file",
	}
	ErrCreateZipFile = errorType{
		Code:    "CLI-0017",
		Message: "Failed to create zip file",
	}
	ErrOpenFile = errorType{
		Code:    "CLI-0018",
		Message: "Failed to open file",
	}
	ErrLoadTemplates = errorType{
		Code:    "CLI-0019",
		Message: "Failed to load templates",
	}
	ErrAuthenticateOtelServer = errorType{
		Code:    "CLI-0020",
		Message: "Failed to authenticate with otel server",
	}
)

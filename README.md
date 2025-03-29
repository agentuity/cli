<div align="center">
    <img src="https://raw.githubusercontent.com/agentuity/cli/main/.github/Agentuity.png" alt="Agentuity" width="100"/> <br/>
    <strong>Build Agents, Not Infrastructure</strong> <br/>
<br />
<a href="https://github.com/agentuity/cli/releases"><img alt="Release version" src="https://img.shields.io/github/v/release/agentuity/cli"></a>
<a href="https://github.com/agentuity/sdk-js/blob/main/README.md"><img alt="License" src="https://badgen.now.sh/badge/license/Apache-2.0"></a>
<a href="https://discord.gg/vtn3hgUfuc"><img alt="Join the community on Discord" src="https://img.shields.io/discord/1332974865371758646.svg?style=flat"></a>
</div>
</div>

# Agentuity CLI


The command line tools for the Agentuity Agent Cloud Platform.  These tools are used to build, manage, and deploy Agents to the Agentuity platform.

## Installation

You can install the CLI using the install script:

```bash
curl -fsSL https://agentuity.sh/install.sh | sh
```

If you are on a Mac, you can install the CLI using Homebrew:

```bash
 brew install agentuity/tap/agentuity
```

For Windows, you can install the CLI using the install script:

```bash
irm https://agentuity.sh/install.ps1 | iex
```

For other platforms, please download the binary from the [Releases](https://github.com/agentuity/cli/releases) page.

## Upgrade

If you have already installed the CLI, you can upgrade to the latest version using the upgrade command:

```bash
agentuity upgrade
```

## Usage

```bash
agentuity --help
```

## Usage

The Agentuity CLI provides a comprehensive set of commands to help you build, manage, and deploy Agents. Here's an overview of the available commands:

### Basic Commands

```bash
# Display help information
agentuity --help

# Check the CLI version
agentuity version

# Login to the Agentuity Cloud Platform
agentuity login
```

### Project Management

```bash
# Create a new project
agentuity create [name]
# or
agentuity project create [name] [--dir <directory>] [--provider <provider>]

# List all projects
agentuity project list

# Delete one or more projects
agentuity project delete
```

### Agent Management

```bash
# Create a new agent
agentuity agent create

# List all Agents in the project
agentuity agent list

# Delete one or more Agents
agentuity agent delete
```

### Development and Deployment

```bash
# Run the development server
agentuity dev

# Deploy your project to the cloud
agentuity deploy
# or
agentuity cloud deploy [--dir <directory>]
```

### Other Commands

```bash
# Environment related commands
agentuity env

# Authentication and authorization
agentuity auth
```

For more detailed information about any command, you can use:

```bash
agentuity [command] --help
```

## Development

### Error Code System

The CLI uses a centralized error code system to provide consistent error messages and codes. Error codes are defined in `error_codes.yaml` at the root of the project and are automatically generated into Go code.

To add a new error code:

1. Edit `error_codes.yaml` and add a new entry with a unique code and descriptive message
2. Run `go generate ./...` to update the Go code
3. Use the generated error type in your code with `errsystem.New(errsystem.ErrYourError, err)`

For more details, see the [Error Code System documentation](tools/README.md).

## Templates

The CLI provides a set of templates for building Agents. These templates are used to create new Agent projects and are stored in the [agentuity/templates](https://github.com/agentuity/templates) repository. See the [Templates README](https://github.com/agentuity/templates/blob/main/README.md) for more details.


## License

See the [LICENSE](LICENSE.md) file for details.

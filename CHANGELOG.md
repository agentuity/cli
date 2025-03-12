# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.0.58] - 2025-03-12

### Fixed
- Fix filepath issues by converting to localized path separators for Windows compatibility ([#80](https://github.com/agentuity/cli/pull/80)) (@jhaynie)

## [v0.0.57] - 2025-03-12

### Added
- Add Python cursor rules files ([#75](https://github.com/agentuity/cli/pull/75))
- Add support for remembering new project preferences ([#74](https://github.com/agentuity/cli/pull/74))

### Fixed
- Fix issue when importing with an existing env ([#78](https://github.com/agentuity/cli/pull/78))

## [v0.0.56] - 2025-03-12

### Added
- Project Import on Cloud Deploy: Added functionality to automatically import projects when deploying to the cloud if the project ID is not found or when using a new template ([#73](https://github.com/agentuity/cli/pull/73))
- Added project import command (`agentuity project import`)
- Added project import checks during cloud deployment
- Added project import checks during development mode
- Added project import checks for new agent creation

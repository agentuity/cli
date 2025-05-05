# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.0.117] - 2025-05-05

### Fixed
- Fixed typo in CI flag name (changed "ci-messsage" to "ci-message") ([#277](https://github.com/agentuity/cli/pull/277))

## [v0.0.115] - 2025-05-02

### Added
- Added mono repofix ([#267](https://github.com/agentuity/cli/pull/267))

### Changed
- Add flags for deployment data overwrite from github action ([#266](https://github.com/agentuity/cli/pull/266))

### Fixed
- Allow non-admin users to uninstall CLI without admin privileges ([#264](https://github.com/agentuity/cli/pull/264))

## [v0.0.114] - 2025-05-01

### Fixed
- Don't call close handler if conn is nil ([#255](https://github.com/agentuity/cli/pull/255))
- Fix 'text file busy' error during CLI updates ([#254](https://github.com/agentuity/cli/pull/254))
- Windows: improve the output for windows upgrade ([#253](https://github.com/agentuity/cli/pull/253))
- Fix PowerShell installation issues in install.ps1 ([#257](https://github.com/agentuity/cli/pull/257))
- DevMode: Make sure to terminate child processes ([#259](https://github.com/agentuity/cli/pull/259))
- Don't error if deliberate restart ([#260](https://github.com/agentuity/cli/pull/260))
- Set ALLUSERS=0 for non-admin installations to ensure proper installation to AppData/Local/Agentuity ([#261](https://github.com/agentuity/cli/pull/261))

### Added
- Update install.sh to support /home/ubuntu/.bin and prompt for shell reload ([#258](https://github.com/agentuity/cli/pull/258))
- Add breaking change for new Python SDK ([#256](https://github.com/agentuity/cli/pull/256))

## [v0.0.113] - 2025-04-29

### Added
- Make sure agent create has a reference to the template so we can reference it in interpolation ([#251](https://github.com/agentuity/cli/pull/251))

### Changed
- DevMode: debounce hot reloads ([#250](https://github.com/agentuity/cli/pull/250))

## [v0.0.112] - 2025-04-29

### Fixed
- Enhance Windows MSI upgrade with better fallbacks and error handling ([#249](https://github.com/agentuity/cli/pull/249))

## [v0.0.111] - 2025-04-29

### Fixed
- Fix PowerShell installation error with global drive ([#246](https://github.com/agentuity/cli/pull/246))
- Improve hot reload stability ([#245](https://github.com/agentuity/cli/pull/245))
- Fix Windows upgrade process to uninstall existing CLI before installation ([#244](https://github.com/agentuity/cli/pull/244))

## [v0.0.110] - 2025-04-29

### Fixed
- More logging and cleanup for dev server startup, more safe filename fixes for python which is stricter ([#242](https://github.com/agentuity/cli/pull/242))

## [v0.0.109] - 2025-04-29

### Fixed
- Fix issue with windows startup for devmode ([#240](https://github.com/agentuity/cli/pull/240))
  - Increased wait time for devmode on Windows
  - Added more logging
  - Fixed Windows path escape issue in agents JSON
  - Decreased attempt duration

## [v0.0.107] - 2025-04-29

### Fixed
- DevMode: connect to ipv4 loopback explicitly ([#237](https://github.com/agentuity/cli/pull/237))

## [v0.0.106] - 2025-04-26

### Changed
- Use update not upgrade ([#235](https://github.com/agentuity/cli/pull/235))
- Some Node libraries which have already been bundled conflict with our bundle require shim ([#233](https://github.com/agentuity/cli/pull/233))

### Documentation
- Update changelog for v0.0.105 ([#232](https://github.com/agentuity/cli/pull/232))
- Fix doc link

## [v0.0.105] - 2025-04-25

### Changed
- Temporarily revert the agent rename detection ([#231](https://github.com/agentuity/cli/pull/231))
- Revert "temporarily comment out the new sdk upgrade requirement until ready" ([#229](https://github.com/agentuity/cli/pull/229))
- Update the dev help output to use the direct agent route instead of legacy route ([#224](https://github.com/agentuity/cli/pull/224))

## [v0.0.104] - 2025-04-24

### Changed
- Small tweaks around help and error dialog ([#227](https://github.com/agentuity/cli/pull/227))

### Fixed
- Fix regression in devmode input json using new binary protocol ([#228](https://github.com/agentuity/cli/pull/228))
- Add error message for JS SDK breaking change ([#225](https://github.com/agentuity/cli/pull/225))
- Project Name uniqueness check should be within project not any project in the users org ([#223](https://github.com/agentuity/cli/pull/223))
- Add a more helpful error message when dev command cannot validate the project ([#222](https://github.com/agentuity/cli/pull/222))
- Better handling when you rename an agent and attempt to deploy ([#221](https://github.com/agentuity/cli/pull/221))

### Documentation
- Update changelog for v0.0.103 ([#220](https://github.com/agentuity/cli/pull/220))

## [v0.0.103] - 2025-04-23

### Fixed
- Fix dev mode for new sdk ([#219](https://github.com/agentuity/cli/pull/219))
- A better error messages when trying to load a project ([#218](https://github.com/agentuity/cli/pull/218))

## [v0.0.102] - 2025-04-21

### Fixed
- Don't fail if MCP detection fails for any reason on create project ([#216](https://github.com/agentuity/cli/pull/216))

## [v0.0.101] - 2025-04-19

### Fixed
- Fix unzip function to properly close file handles ([#215](https://github.com/agentuity/cli/pull/215))

## [v0.0.100] - 2025-04-19

### Changed
- Be smart about remembering the last project ([#212](https://github.com/agentuity/cli/pull/212))
- Hide websocket-id flag from CLI help text ([#211](https://github.com/agentuity/cli/pull/211))

### Documentation
- Update changelog for v0.0.99 ([#210](https://github.com/agentuity/cli/pull/210))

## [v0.0.99] - 2025-04-18

### Changed
- Add a better error message on new project by using a dialog component ([#209](https://github.com/agentuity/cli/pull/209))

## [v0.0.98] - 2025-04-18

### Changed
- Add exponential backoff for agent welcome connection with 30s max time ([#207](https://github.com/agentuity/cli/pull/207))

## [v0.0.97] - 2025-04-17

### Fixed
- Fix issue with too many files error ([#205](https://github.com/agentuity/cli/pull/205))
- Fixed small error (55996e3)

### Changed
- Bump golang.org/x/net from 0.36.0 to 0.38.0 ([#204](https://github.com/agentuity/cli/pull/204))

### Documentation
- Update changelog for v0.0.96 ([#203](https://github.com/agentuity/cli/pull/203))

## [v0.0.96] - 2025-04-16

### Fixed
- Guard against conn being nil ([e095c09](https://github.com/agentuity/cli/commit/e095c09))
- Only set step cursor on page 1 ([#202](https://github.com/agentuity/cli/pull/202))

## [v0.0.95] - 2025-04-16

### Added
- Add retries to HTTP client ([#200](https://github.com/agentuity/cli/pull/200))

### Changed
- Attempt to have better UX handling of upgrade checks ([#199](https://github.com/agentuity/cli/pull/199))
- Template Improvements ([#198](https://github.com/agentuity/cli/pull/198))

### Documentation
- Update changelog for v0.0.94 ([#197](https://github.com/agentuity/cli/pull/197))

## [v0.0.94] - 2025-04-16

### Fixed
- Fix for mismatched lockfile when package.json version doesn't match the bun lock file by removing the --frozen-lockfile flag ([#196](https://github.com/agentuity/cli/pull/196))

## [v0.0.93] - 2025-04-16

### Changed
- Improve TUI semantics ([#193](https://github.com/agentuity/cli/pull/193))

### Fixed
- Add more debug logging around CI bundling for github app ([#194](https://github.com/agentuity/cli/pull/194))

### Documentation
- Update changelog for v0.0.92 ([#192](https://github.com/agentuity/cli/pull/192))

## [v0.0.92] - 2025-04-15

### Fixed
- Fix the Git URL to rewrite to https ([#190](https://github.com/agentuity/cli/pull/190))

### Changed
- Add hyperlinks to older release versions in CHANGELOG.md ([#191](https://github.com/agentuity/cli/pull/191))
- Update changelog for v0.0.91 ([#189](https://github.com/agentuity/cli/pull/189))

## [v0.0.91] - 2025-04-14

### Fixed
- Fix go-common flag issue with overriding log level from env and add more debug to bundle ([#188](https://github.com/agentuity/cli/pull/188))

## [v0.0.90] - 2025-04-14

### Added
- Add support for managing API Keys from CLI ([#186](https://github.com/agentuity/cli/pull/186))

### Fixed
- Make sure we set the working directory when running the project dev command since we could be using --dir

## [v0.0.89] - 2025-04-10

### Added
- Add CLI Signup Flow ([#182](https://github.com/agentuity/cli/pull/182))

### Fixed
- Fix macOS segfault during reinstallation ([#183](https://github.com/agentuity/cli/pull/183))
- Smart login or setup ([#184](https://github.com/agentuity/cli/pull/184))

## [v0.0.88] - 2025-04-08

### Added
- Webhook instructions ([#179](https://github.com/agentuity/cli/pull/179))

### Changed
- Proxy GitHub public APIs ([#180](https://github.com/agentuity/cli/pull/180))
- Small improvements on devmode

### Fixed
- Make it clear that the webhook is a POST ([#178](https://github.com/agentuity/cli/pull/178))
- If node_modules or .venv/lib directory are missing when bundling, force install ([#177](https://github.com/agentuity/cli/pull/177))

## [v0.0.87] - 2025-04-08

### Fixed
- Fix regression in step 2 (new project) related to cursor selection ([234b330](https://github.com/agentuity/cli/commit/234b3307d1fd96005d4f656ab319d438e7b60626))

## [v0.0.86] - 2025-04-07

### Added
- Add Clone Repo step ([#171](https://github.com/agentuity/cli/pull/171))
- Add Agent Welcome on DevMode ([#172](https://github.com/agentuity/cli/pull/172))

### Changed
- Totally re-write the TUI for the new project ([#170](https://github.com/agentuity/cli/pull/170))
- Better upgrade handling ([#174](https://github.com/agentuity/cli/pull/174))

### Fixed
- Fix crewai installation issue (exit status 130) ([#169](https://github.com/agentuity/cli/pull/169))
- Make sure command is executed with a context ([#173](https://github.com/agentuity/cli/pull/173))

## [v0.0.85] - 2025-04-05

### Added
- Added project id on messages for devmode ([#167](https://github.com/agentuity/cli/pull/167))

## [v0.0.84] - 2025-04-03

### Fixed
- Fixed bundler version not having the right cli version ([#165](https://github.com/agentuity/cli/pull/165))

## [v0.0.83] - 2025-04-01

### Changed
- Devmode fixes and improvements ([#164](https://github.com/agentuity/cli/pull/164))

## [v0.0.82] - 2025-03-30

### Fixed
- Small improvement for homebrew upgrade and fix upgrade url prefix ([#163](https://github.com/agentuity/cli/pull/163))

## [v0.0.81] - 2025-03-28

### Changed
- Use transport url for transport url ([#162](https://github.com/agentuity/cli/pull/162))

## [v0.0.80] - 2025-03-27

### Changed
- Use windows-latest instead of windows for build environment

## [v0.0.79] - 2025-03-26

### Fixed
- Fix version comparison in upgrade command to handle v prefix ([#158](https://github.com/agentuity/cli/pull/158))

## [v0.0.78] - 2025-03-26

### Added
- Add the new env AGENTUITY_TRANSPORT_URL for the bundler and use the new gateway URL ([#155](https://github.com/agentuity/cli/pull/155))

## [v0.0.77] - 2025-03-26

### Changed
- Use app/api url for api url ([#152](https://github.com/agentuity/cli/pull/152))

## [v0.0.76] - 2025-03-26

### Changed
- Use a different key for authentication

## [v0.0.75] - 2025-03-26

### Changed
- Try and use github runner for builds
## [v0.0.74] - 2025-03-25

### Added
- JSON Schema for agentuity.yaml file ([#126](https://github.com/agentuity/cli/pull/126), [#127](https://github.com/agentuity/cli/pull/127))
- MCP Support ([#121](https://github.com/agentuity/cli/pull/121))

### Fixed
- Windows installer and MCP fixes ([#129](https://github.com/agentuity/cli/pull/129))
- Improved dev command shutdown to ensure all child processes are terminated ([#128](https://github.com/agentuity/cli/pull/128))
- Fixed issue when dev port is taken by automatically choosing another port ([#125](https://github.com/agentuity/cli/pull/125))
- Git deployment metadata fix ([#120](https://github.com/agentuity/cli/pull/120))

### Changed
- GitHub improvements ([#123](https://github.com/agentuity/cli/pull/123))

## [v0.0.73] - 2025-03-21

### Fixed
- Python: force --env-file when running in devmode ([#118](https://github.com/agentuity/cli/pull/118))

### Changed
- place .env on another line to be safe

## [v0.0.71] - 2025-03-20

### Changed
- Pass on dir flag when doing bundle --deploy ([#115](https://github.com/agentuity/cli/pull/115))

## [v0.0.70] - 2025-03-19

### Added
- Initial Implementation of Automatic Version checking ([#113](https://github.com/agentuity/cli/pull/113))

## [v0.0.69] - 2025-03-19

### Fixed
- Handle auth failure better ([#112](https://github.com/agentuity/cli/pull/112))

### Changed
- Move internal/tui package to use go-common/tui package so we can reuse ([#111](https://github.com/agentuity/cli/pull/111))
- Improve Project List View and Auth Whoami ([#110](https://github.com/agentuity/cli/pull/110))

## [v0.0.68] - 2025-03-19

### Fixed
- Better handle user interruption errors ([#109](https://github.com/agentuity/cli/pull/109))

## [v0.0.67] - 2025-03-19

### Added
- Force new project to always use the latest sdk ([#108](https://github.com/agentuity/cli/pull/108))

### Fixed
- DevMode: cleanup payload to make sure we keep it as []byte vs using string so we always transmit in base64 w/o recoding by accident ([#107](https://github.com/agentuity/cli/pull/107))

## [v0.0.66] - 2025-03-17

### Changed
- Rename devmode ([#106](https://github.com/agentuity/cli/pull/106))
- Dev Mode: deterministic room id ([#63](https://github.com/agentuity/cli/pull/63))

## [v0.0.65] - 2025-03-17

### Fixed
- Be smarter on error message of JS when running node directly ([#105](https://github.com/agentuity/cli/pull/105))
- Add environment variable checks to Python boot.py ([#103](https://github.com/agentuity/cli/pull/103))

### Added
- Added project id on for matt ([#104](https://github.com/agentuity/cli/pull/104))

## [v0.0.64] - 2025-03-16

### Added
- Add README template for JavaScript projects ([#102](https://github.com/agentuity/cli/pull/102))

## [v0.0.63] - 2025-03-16

### Changed
- Improve CTRL-C cancel, always send user-agent with version for API requests ([#101](https://github.com/agentuity/cli/pull/101))

## [v0.0.62] - 2025-03-16

### Fixed
- Fix change in signature with request.text -> request.data.text ([#100](https://github.com/agentuity/cli/pull/100))

### Added
- Add Long property documentation to all CLI commands ([#99](https://github.com/agentuity/cli/pull/99))
- Add traceparent in the error handling logic to aid in debugging issues ([#98](https://github.com/agentuity/cli/pull/98))

## [v0.0.61] - 2025-03-15

### Added
- Add Org Level data encryption for agent source ([#97](https://github.com/agentuity/cli/pull/97))
- Improve missing LLM environment variables ([#95](https://github.com/agentuity/cli/pull/95))

### Fixed
- Don't set AGENTUITY_ENVIRONMENT on the production bundle, let it get set by the infra ([#96](https://github.com/agentuity/cli/pull/96))
- Fix issue with --env-file not getting picked up in node when running dev ([#94](https://github.com/agentuity/cli/pull/94))

### Documentation
- Update changelog for v0.0.60 ([#93](https://github.com/agentuity/cli/pull/93))

## [v0.0.72] - 2025-03-20

### Added
- Added deployment metadata and CI flag for GitHub actions ([#116](https://github.com/agentuity/cli/pull/116))

### Fixed
- Fixed bug in file watcher ([#114](https://github.com/agentuity/cli/pull/114))
- Don't send error reports when using the dev version

## [v0.0.60] - 2025-03-13

### Added
- Add support for new transport domain (agentuity.ai) ([#89](https://github.com/agentuity/cli/pull/89))
- Add profile switching for local development ([#89](https://github.com/agentuity/cli/pull/89))

### Fixed
- Improve agent deletion logic with backup functionality ([#90](https://github.com/agentuity/cli/pull/90))
- Correct .dev domain references ([#91](https://github.com/agentuity/cli/pull/91), [#92](https://github.com/agentuity/cli/pull/92))

## [v0.0.59] - 2025-03-13

### Changed
- Move deployment manifest from `agentuity-deployment.yaml` to `.agentuity/.manifest.yaml` ([#86](https://github.com/agentuity/cli/pull/86))

### Fixed
- Improve UI by showing information banner instead of error when a requirement cannot be met ([#85](https://github.com/agentuity/cli/pull/85))
- Fix development mode issues and environment variable handling for JavaScript environments ([#87](https://github.com/agentuity/cli/pull/87))

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

[v0.0.117]: https://github.com/agentuity/cli/compare/v0.0.116...v0.0.117
[v0.0.116]: https://github.com/agentuity/cli/compare/v0.0.115...v0.0.116
[v0.0.115]: https://github.com/agentuity/cli/compare/v0.0.114...v0.0.115
[v0.0.114]: https://github.com/agentuity/cli/compare/v0.0.113...v0.0.114
[v0.0.113]: https://github.com/agentuity/cli/compare/v0.0.112...v0.0.113
[v0.0.112]: https://github.com/agentuity/cli/compare/v0.0.111...v0.0.112
[v0.0.111]: https://github.com/agentuity/cli/compare/v0.0.110...v0.0.111
[v0.0.110]: https://github.com/agentuity/cli/compare/v0.0.109...v0.0.110
[v0.0.109]: https://github.com/agentuity/cli/compare/v0.0.108...v0.0.109
[v0.0.108]: https://github.com/agentuity/cli/compare/v0.0.107...v0.0.108
[v0.0.107]: https://github.com/agentuity/cli/compare/v0.0.106...v0.0.107
[v0.0.106]: https://github.com/agentuity/cli/compare/v0.0.105...v0.0.106
[v0.0.105]: https://github.com/agentuity/cli/compare/v0.0.104...v0.0.105
[v0.0.104]: https://github.com/agentuity/cli/compare/v0.0.103...v0.0.104
[v0.0.103]: https://github.com/agentuity/cli/compare/v0.0.102...v0.0.103
[v0.0.102]: https://github.com/agentuity/cli/compare/v0.0.101...v0.0.102
[v0.0.101]: https://github.com/agentuity/cli/compare/v0.0.100...v0.0.101
[v0.0.100]: https://github.com/agentuity/cli/compare/v0.0.99...v0.0.100
[v0.0.99]: https://github.com/agentuity/cli/compare/v0.0.98...v0.0.99
[v0.0.98]: https://github.com/agentuity/cli/compare/v0.0.97...v0.0.98
[v0.0.97]: https://github.com/agentuity/cli/compare/v0.0.96...v0.0.97
[v0.0.96]: https://github.com/agentuity/cli/compare/v0.0.95...v0.0.96
[v0.0.95]: https://github.com/agentuity/cli/compare/v0.0.94...v0.0.95
[v0.0.94]: https://github.com/agentuity/cli/compare/v0.0.93...v0.0.94
[v0.0.93]: https://github.com/agentuity/cli/compare/v0.0.92...v0.0.93
[v0.0.92]: https://github.com/agentuity/cli/compare/v0.0.91...v0.0.92
[v0.0.91]: https://github.com/agentuity/cli/compare/v0.0.90...v0.0.91
[v0.0.90]: https://github.com/agentuity/cli/compare/v0.0.89...v0.0.90
[v0.0.89]: https://github.com/agentuity/cli/compare/v0.0.88...v0.0.89
[v0.0.88]: https://github.com/agentuity/cli/compare/v0.0.87...v0.0.88
[v0.0.87]: https://github.com/agentuity/cli/compare/v0.0.86...v0.0.87
[v0.0.86]: https://github.com/agentuity/cli/compare/v0.0.85...v0.0.86
[v0.0.85]: https://github.com/agentuity/cli/compare/v0.0.84...v0.0.85
[v0.0.84]: https://github.com/agentuity/cli/compare/v0.0.83...v0.0.84
[v0.0.83]: https://github.com/agentuity/cli/compare/v0.0.82...v0.0.83
[v0.0.82]: https://github.com/agentuity/cli/compare/v0.0.81...v0.0.82
[v0.0.81]: https://github.com/agentuity/cli/compare/v0.0.80...v0.0.81
[v0.0.80]: https://github.com/agentuity/cli/compare/v0.0.79...v0.0.80
[v0.0.79]: https://github.com/agentuity/cli/compare/v0.0.78...v0.0.79
[v0.0.78]: https://github.com/agentuity/cli/compare/v0.0.77...v0.0.78
[v0.0.77]: https://github.com/agentuity/cli/compare/v0.0.76...v0.0.77
[v0.0.76]: https://github.com/agentuity/cli/compare/v0.0.75...v0.0.76
[v0.0.75]: https://github.com/agentuity/cli/compare/v0.0.74...v0.0.75
[v0.0.74]: https://github.com/agentuity/cli/compare/v0.0.73...v0.0.74
[v0.0.73]: https://github.com/agentuity/cli/compare/v0.0.72...v0.0.73
[v0.0.72]: https://github.com/agentuity/cli/compare/v0.0.71...v0.0.72
[v0.0.71]: https://github.com/agentuity/cli/compare/v0.0.70...v0.0.71
[v0.0.70]: https://github.com/agentuity/cli/compare/v0.0.69...v0.0.70
[v0.0.69]: https://github.com/agentuity/cli/compare/v0.0.68...v0.0.69
[v0.0.68]: https://github.com/agentuity/cli/compare/v0.0.67...v0.0.68
[v0.0.67]: https://github.com/agentuity/cli/compare/v0.0.66...v0.0.67
[v0.0.66]: https://github.com/agentuity/cli/compare/v0.0.65...v0.0.66
[v0.0.65]: https://github.com/agentuity/cli/compare/v0.0.64...v0.0.65
[v0.0.64]: https://github.com/agentuity/cli/compare/v0.0.63...v0.0.64
[v0.0.63]: https://github.com/agentuity/cli/compare/v0.0.62...v0.0.63
[v0.0.62]: https://github.com/agentuity/cli/compare/v0.0.61...v0.0.62
[v0.0.61]: https://github.com/agentuity/cli/compare/v0.0.60...v0.0.61
[v0.0.60]: https://github.com/agentuity/cli/compare/v0.0.59...v0.0.60
[v0.0.59]: https://github.com/agentuity/cli/compare/v0.0.58...v0.0.59
[v0.0.58]: https://github.com/agentuity/cli/compare/v0.0.57...v0.0.58
[v0.0.57]: https://github.com/agentuity/cli/compare/v0.0.56...v0.0.57
[v0.0.56]: https://github.com/agentuity/cli/compare/v0.0.55...v0.0.56

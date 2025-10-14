# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.0.175] - 2025-10-13

### Changed
- Bump go-common to remove gravity log ([#466](https://github.com/agentuity/cli/pull/466))

## [v0.0.174] - 2025-10-10

### Changed
- Updated go-common dependency to v1.0.103

## [v0.0.173] - 2025-10-10

### Added
- Add support for automatic bundling of native external modules ([#458](https://github.com/agentuity/cli/pull/458))

### Changed
- DevMode 2.0 ([#449](https://github.com/agentuity/cli/pull/449))

### Fixed
- Fix duplicate Agentuity keys in .env file creation ([#459](https://github.com/agentuity/cli/pull/459))


## [v0.0.172] - 2025-10-10

### Added
- Added patching logic for prompt metadata ([#462](https://github.com/agentuity/cli/pull/462))


## [v0.0.171] - 2025-10-03

### Added
- Multi-file YAML prompt support ([#460](https://github.com/agentuity/cli/pull/460))

### Changed
- Function signature modifications and code generation enhancements ([#460](https://github.com/agentuity/cli/pull/460))


## [v0.0.170] - 2025-10-02

### Added
- Public key encryption for self-hosted deployments and new infra ([#429](https://github.com/agentuity/cli/pull/429))

## [v0.0.169] - 2025-10-02

### Added
- Prompt eval changes ([#455](https://github.com/agentuity/cli/pull/455))
- Added feature flag to the CLI config ([#456](https://github.com/agentuity/cli/pull/456))


## [v0.0.168] - 2025-09-24

### Added
- Add support for importing: yaml, json, txt, png, gif, jpg, svg, webp, md, csv, pdf, sql, xml ([#452](https://github.com/agentuity/cli/pull/452))
- Onboard nudges on next steps. Changed shell completion failure from warn to ohai so it doesn't look like installation failed. ([#454](https://github.com/agentuity/cli/pull/454))


## [v0.0.167] - 2025-09-24

### Added
- [AGENT-684] Check if zsh is installed before adding autocomplete in the CLI ([#450](https://github.com/agentuity/cli/pull/450))
- [AGENT-628] Unit tests ([#441](https://github.com/agentuity/cli/pull/441))
- feat: automatically add AGENTUITY_SDK_KEY and AGENTUITY_PROJECT_KEY to .env file when running dev command ([#442](https://github.com/agentuity/cli/pull/442))

### Changed
- Dont sort releases by commit msg ([#447](https://github.com/agentuity/cli/pull/447))
- [AGENT-628] prevent local development env files from syncing to production ([#440](https://github.com/agentuity/cli/pull/440))

### Fixed
- Fix npm workspaces ([#451](https://github.com/agentuity/cli/pull/451))
- Fix 'Press any key to continue' to accept any key, not just Enter ([#445](https://github.com/agentuity/cli/pull/445))

## [v0.0.166] - 2025-08-28

### Added
- Adds support for pnpm as a bundler ([#438](https://github.com/agentuity/cli/pull/438))

### Fixed
- bump go-common to get light mode color fixes

## [v0.0.165] - 2025-08-18

### Changed
- Exclude the .jj folder automatically ([#434](https://github.com/agentuity/cli/pull/434))
- Changes to make teams work ([#432](https://github.com/agentuity/cli/pull/432))

## [v0.0.164] - 2025-08-12

### Added
- Allow setup.sh in case we need to run some script in docker image ([#427](https://github.com/agentuity/cli/pull/427))

### Changed
- Add more debugging around new project to debug failure ([#430](https://github.com/agentuity/cli/pull/430))
- Make it scroll with arrows as well ([#428](https://github.com/agentuity/cli/pull/428))

## [v0.0.163] - 2025-07-22

### Added
- Support for dry-run command ([#425](https://github.com/agentuity/cli/pull/425))

## [v0.0.162] - 2025-07-18

### Fixed
- Fix issue with env on deploy asking for setting when set. Suppress some logs ([#424](https://github.com/agentuity/cli/pull/424))
- Added an option to open login OTP with enter ([#420](https://github.com/agentuity/cli/pull/420))

## [v0.0.161] - 2025-07-16

### Added
- Support for multiple deployment tags in bundle command ([#422](https://github.com/agentuity/cli/pull/422))

## [v0.0.160] - 2025-07-15

### Added
- Support for preview environments ([#418](https://github.com/agentuity/cli/pull/418))

## [v0.0.159] - 2025-07-15

### Fixed
- Fix issue with deployment error not having a write to write out the error ([#416](https://github.com/agentuity/cli/pull/416))

## [v0.0.158] - 2025-07-08

### Fixed
- Bun: re-generate the lock file when we go to install for the first time or in CI ([#414](https://github.com/agentuity/cli/pull/414))

## [v0.0.157] - 2025-07-07

### Fixed
- Bun/Node: remove the --no-save flag which was causing issues during package installation ([#412](https://github.com/agentuity/cli/pull/412))

## [v0.0.156] - 2025-07-03

### Added
- Deployment: add new deployment options ([#410](https://github.com/agentuity/cli/pull/410))

### Fixed
- [GITHUB-338] Do not create a git repo if we're already in a git repo ([#406](https://github.com/agentuity/cli/pull/406))
- BUG FIX compare with original, not masked ([#409](https://github.com/agentuity/cli/pull/409))

## [v0.0.155] - 2025-06-30

### Added
- List deployments ([#405](https://github.com/agentuity/cli/pull/405))

### Fixed
- Fix upgrade success message styling ([#404](https://github.com/agentuity/cli/pull/404))

### Security
- Don't upload masked values to backend ([#407](https://github.com/agentuity/cli/pull/407))

## [v0.0.154] - 2025-06-26

### Added
- Add project delete in headless mode with support for specifying project IDs as arguments and --force flag ([#402](https://github.com/agentuity/cli/pull/402))

## [v0.0.153] - 2025-06-24

### Changed
- When running dev mode, make sure we have a .env.development file ([#400](https://github.com/agentuity/cli/pull/400))

## [v0.0.152] - 2025-06-24

### Fixed
- Railgurd the user from creating a project in an existing project directory ([#396](https://github.com/agentuity/cli/pull/396))
- Fix regression in hot reload for new bun/node templates ([#398](https://github.com/agentuity/cli/pull/398))

## [v0.0.151] - 2025-06-18

### Changed
- Switch to using a more compatible docker internal hostname that works cross platform ([#394](https://github.com/agentuity/cli/pull/394))
- Split up the upload vs deploy status messages ([#395](https://github.com/agentuity/cli/pull/395))

## [v0.0.150] - 2025-06-13

### Added
- Add groq patch module ([#390](https://github.com/agentuity/cli/pull/390))

### Fixed
- Complete JSONC parsing fix for MCPConfig.UnmarshalJSON method ([#388](https://github.com/agentuity/cli/pull/388))
- Fix install script race condition by eliminating separate HEAD request ([#389](https://github.com/agentuity/cli/pull/389))

## [v0.0.149] - 2025-06-13

### Fixed
- Fix Amp MCP config parsing to support JSON with comments ([#386](https://github.com/agentuity/cli/pull/386))
- Add more flexible ignore matching for full ignore rule ([#385](https://github.com/agentuity/cli/pull/385))

## [v0.0.148] - 2025-06-11

### Changed
- More normalized Python project naming ([#383](https://github.com/agentuity/cli/pull/383))
- Add AMP MCP support ([#382](https://github.com/agentuity/cli/pull/382))

## [v0.0.147] - 2025-06-11

### Changed
- Skip TypeScript type checking in production builds to improve build performance ([#380](https://github.com/agentuity/cli/pull/380))

## [v0.0.146] - 2025-06-11

### Changed
- Need to use the changed directory when using last known project ([#358](https://github.com/agentuity/cli/pull/358))

## [v0.0.145] - 2025-06-10

### Added
- TypeScript type checking is now performed automatically during the build process if a TypeScript compiler is present ([#376](https://github.com/agentuity/cli/pull/376))

### Changed
- Enhanced file watcher to use ignore rules for filtering, improving reliability and performance ([#376](https://github.com/agentuity/cli/pull/376))
- Streamlined development server restart logic for smoother and more predictable restarts ([#376](https://github.com/agentuity/cli/pull/376))
- Centralized ignore rules creation for project deployments to simplify configuration management ([#376](https://github.com/agentuity/cli/pull/376))

### Fixed
- Improved error message handling when a required project file is missing, ensuring messages are displayed appropriately based on terminal capabilities ([#375](https://github.com/agentuity/cli/pull/375))
- Improved file and directory ignore handling, ensuring that common development files and directories (e.g., swap files, backup files, node_modules, virtual environments) are consistently excluded across all relevant features ([#376](https://github.com/agentuity/cli/pull/376))

## [v0.0.144] - 2025-06-03

### Fixed
- Fixed terminal cursor disappearing after breaking change error by returning error instead of os.Exit(1) ([#373](https://github.com/agentuity/cli/pull/373))
- Fixed issue with agent casing not being considered ([#372](https://github.com/agentuity/cli/pull/372))
- Fixed filtering of environment variables and secrets that are internal ([#371](https://github.com/agentuity/cli/pull/371))

## [v0.0.143] - 2025-05-30

### Changed
- Improvements around packaging ([#368](https://github.com/agentuity/cli/pull/368))

### Fixed
- [AGENT-258] Use utility function from envutil for better environment variable handling ([#364](https://github.com/agentuity/cli/pull/364))
- Cloned projects now automatically include an .env file ([#369](https://github.com/agentuity/cli/pull/369))

## [v0.0.142] - 2025-05-29

### Changed
- [AGENT-272] Always reinitialize the viewport on resize to avoid invalid state ([#363](https://github.com/agentuity/cli/pull/363))
- MCP: Relaxed types for MCP parsing configs ([#365](https://github.com/agentuity/cli/pull/365))
- Allow SDK to be copied into a project for development purposes ([#366](https://github.com/agentuity/cli/pull/366))

### Fixed
- [AGENT-232] Fixed issue with Python project name by ensuring agent names are lowercase ([#359](https://github.com/agentuity/cli/pull/359))
- Don't run git init when you are already in a git repository ([#362](https://github.com/agentuity/cli/pull/362))

## [v0.0.141] - 2025-05-28

### Added
- Added additional ignore rule defaults for better project management ([#357](https://github.com/agentuity/cli/pull/357))
- Added missing disk units for better user feedback ([#355](https://github.com/agentuity/cli/pull/355))

### Fixed
- Deploy: Fixed error with missing files during zip creation ([#354](https://github.com/agentuity/cli/pull/354))
- Fixed directory handling when using last known project ([#358](https://github.com/agentuity/cli/pull/358))

## [v0.0.140] - 2025-05-27

### Added
- [AGENT-122] Added logs command with filtering and streaming capabilities including tail option, flexible duration parsing, and customizable output formatting ([#342](https://github.com/agentuity/cli/pull/342))
- Added tag and description options to the bundle command for deployment metadata ([#351](https://github.com/agentuity/cli/pull/351))
- Added --org-id filter to project list and delete commands for organization-specific operations ([#350](https://github.com/agentuity/cli/pull/350))

### Fixed
- Fixed mono repository support by improving SDK resolution process with better logging for directory traversal ([#352](https://github.com/agentuity/cli/pull/352))
- DevMode: Fixed sourcemap resolution when node_modules is outside the project directory ([#348](https://github.com/agentuity/cli/pull/348))

## [v0.0.139] - 2025-05-24

### Fixed
- Fixed Bun sourcemap shim issue to improve source map support for projects using the "bunjs" runtime ([#346](https://github.com/agentuity/cli/pull/346))

## [v0.0.138] - 2025-05-23

### Changed
- DevMode: Removed TUI (Terminal User Interface) in favor of a simpler logging approach ([#344](https://github.com/agentuity/cli/pull/344))

### Fixed
- Fixed prepareStackTrace error handling and improved DevMode logging ([#343](https://github.com/agentuity/cli/pull/343))
- Added better handling for Ctrl+C in DevMode ([#343](https://github.com/agentuity/cli/pull/343))


## [v0.0.137] - 2025-05-23

### Changed
- Updated Discord community invite links across the CLI, TUI, README, and error system ([#339](https://github.com/agentuity/cli/pull/339))
- Modified prompt in environment handling to separate informational messages from interactive questions ([#339](https://github.com/agentuity/cli/pull/339))
- Updated dependency `github.com/agentuity/go-common` from v1.0.60 to v1.0.64 ([#339](https://github.com/agentuity/cli/pull/339))

### Fixed
- Fixed DevMode: Removed "server" and "force" flags and all related logic, including environment file processing ([#339](https://github.com/agentuity/cli/pull/339))

## [v0.0.136] - 2025-05-22

### Added
- Add copy attributes to project when importing ([#332](https://github.com/agentuity/cli/pull/332))
- Quality of life improvement: if disk requested is smaller than needed, will tell you and potentially adjust ([#330](https://github.com/agentuity/cli/pull/330))
- [AGENT-130] Delete and Roll Back deployments ([#313](https://github.com/agentuity/cli/pull/313))

## [v0.0.135] - 2025-05-22

### Added
- [AGENT-130] Added deployment management features with rollback and delete commands ([#313](https://github.com/agentuity/cli/pull/313))
- Added disk size validation during bundling for JavaScript and Python projects with auto-adjustment option ([#330](https://github.com/agentuity/cli/pull/330))

## [v0.0.134] - 2025-05-22

### Fixed
- Python: improve devmode logging with support for additional log prefixes ([#328](https://github.com/agentuity/cli/pull/328))
- Fixed handling of Python package versions containing "+" in pre-release builds ([#328](https://github.com/agentuity/cli/pull/328))
- Fixed headless import by correcting flag name from "apikey" to "api-key" ([#327](https://github.com/agentuity/cli/pull/327))

## [v0.0.133] - 2025-05-21

### Changed
- Removed Windows from the build matrix in CI workflow ([73fe98b](https://github.com/agentuity/cli/commit/73fe98bca31f3864a1028c379b29aac6cf36350f))

### Added
- DevMode: Allow the API to return a preferred server ([#325](https://github.com/agentuity/cli/pull/325))

### Fixed
- DevMode: Improved logging output and sourcemap support ([#321](https://github.com/agentuity/cli/pull/321))
- [AGENT-209] Refactor adding env vars from file ([#324](https://github.com/agentuity/cli/pull/324))

## [v0.0.132] - 2025-05-21

### Fixed
- [AGENT-166] Add dev mode flag and improved error handling during server streaming operations ([#322](https://github.com/agentuity/cli/pull/322))

## [v0.0.131] - 2025-05-20

### Added
- Added headless import functionality for non-interactive project imports ([#318](https://github.com/agentuity/cli/pull/318))

### Fixed
- Fixed release workflow by removing invalid subject-checksums-type parameter ([#319](https://github.com/agentuity/cli/pull/319))

## [v0.0.130] - 2025-05-20

### Fixed
- DevMode: remove mouse tracking as it caused the inability to copy/paste in the terminal ([#316](https://github.com/agentuity/cli/pull/316))

## [v0.0.129] - 2025-05-20

### Changed
- [AGENT-169] Expose framework to the UI ([#295](https://github.com/agentuity/cli/pull/295))

## [v0.0.128] - 2025-05-18

### Fixed
- DevMode: automatic reconnect if losing connection to echo server ([#308](https://github.com/agentuity/cli/pull/308))

## [v0.0.127] - 2025-05-18

### Fixed
- Added better logging on startup, make sure we kill server if healthcheck fails, wait longer ([#306](https://github.com/agentuity/cli/pull/306))


## [v0.0.126] - 2025-05-18

### Fixed
- DevMode: Fixed issue when using short agent ID wasn't going upstream ([#304](https://github.com/agentuity/cli/pull/304))

## [v0.0.125] - 2025-05-18

### Fixed
- Fixed regression with transport having no IO (public) ([#302](https://github.com/agentuity/cli/pull/302))

## [v0.0.124] - 2025-05-17

### Added
- Mouse support for developer UI (scrolling and log selection) ([#300](https://github.com/agentuity/cli/pull/300))
- Agent welcome messages and optional prompts for richer metadata ([#300](https://github.com/agentuity/cli/pull/300))
- Support for non-TUI mode in VS Code terminals and pipe environments ([#300](https://github.com/agentuity/cli/pull/300))

### Changed
- Renamed interface label from "Dashboard" to "DevMode" for clarity ([#300](https://github.com/agentuity/cli/pull/300))
- Enhanced log display with timestamps and improved formatting ([#300](https://github.com/agentuity/cli/pull/300))
- Don't use alt screen so content is preserved on exit ([#300](https://github.com/agentuity/cli/pull/300))
- Modified CI workflows to ignore documentation-only PRs ([#300](https://github.com/agentuity/cli/pull/300))
- Updated port selection logic for dev server and agent testing ([#300](https://github.com/agentuity/cli/pull/300))

### Fixed
- Fixed port binding conflicts when running multiple agents ([#300](https://github.com/agentuity/cli/pull/300))
- Fixed escape key behavior in main screen ([#300](https://github.com/agentuity/cli/pull/300))
- Fixed log filtering issue ([#300](https://github.com/agentuity/cli/pull/300))
- Fixed regression in welcome prompt not showing up ([#300](https://github.com/agentuity/cli/pull/300))

## [v0.0.123] - 2025-05-17

### Fixed
- Auto switch to local echo if using localhost, fix terminal reset issues ([#298](https://github.com/agentuity/cli/pull/298))

## [v0.0.122] - 2025-05-16

### Changed
- Initial Refactor for DevMode to use new Bridge API and new TUI ([#270](https://github.com/agentuity/cli/pull/270))

## [v0.0.121] - 2025-05-16

### Added
- [AGENT-133] Added "test" command ([#290](https://github.com/agentuity/cli/pull/290))
- [AGENT-129] Multiple tags for a deployment ([#291](https://github.com/agentuity/cli/pull/291))

### Changed
- Add tag and message to deployments in CI ([#293](https://github.com/agentuity/cli/pull/293))

### Fixed
- [AGENT-179] Call the agent from the correct endpoint ([#294](https://github.com/agentuity/cli/pull/294))

## [v0.0.120] - 2025-05-14

### Added
- Add project key for agent comms ([#285](https://github.com/agentuity/cli/pull/285))

### Changed
- Shorten install script, skip prebuilds on breaking change check ([#287](https://github.com/agentuity/cli/pull/287))
- Cleanup: remove old vscode settings, move release to use blacksmith now that we dont need MSI build ([#289](https://github.com/agentuity/cli/pull/289))
- Update copy in upgrade.go for upgrade

### Fixed
- [AGENT-163] Update command for Windows ([#284](https://github.com/agentuity/cli/pull/284))

### Documentation
- Update changelog for v0.0.119 ([#283](https://github.com/agentuity/cli/pull/283))

## [v0.0.119] - 2025-05-08

### Added
- Added path completion to the CLI ([#282](https://github.com/agentuity/cli/pull/282))

### Changed
- Cleanup install script ([#281](https://github.com/agentuity/cli/pull/281))
  - Removed Windows native support (WSL is now recommended)
  - Improved installation testing with Docker
  - Restructured installation script for better maintainability

## [v0.0.118] - 2025-05-06

### Fixed
- Fixed check on dev for Linux by using `sys.IsRunningInsideDocker()` instead of checking for specific Docker files ([#279](https://github.com/agentuity/cli/pull/279))

## [v0.0.117] - 2025-05-05

### Fixed
- Fixed typo in CI flag name (changed "ci-messsage" to "ci-message") ([#277](https://github.com/agentuity/cli/pull/277))

## [v0.0.116] - 2025-05-05

### Fixed
- Missed annotation on GitInfo ([#275](https://github.com/agentuity/cli/pull/275))
- AGENT-29 Check mask value for secrets ([#274](https://github.com/agentuity/cli/pull/274))
- Passing CI logs URL to display GitHub action logs in the UI ([#273](https://github.com/agentuity/cli/pull/273))

### Changed
- Taking a walk to get the git data 🚶‍♂️‍➡️ ([#272](https://github.com/agentuity/cli/pull/272))
- Pass on the git info from deploy to bundle when for --deploy ([#271](https://github.com/agentuity/cli/pull/271))

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

[v0.0.175]: https://github.com/agentuity/cli/compare/v0.0.174...v0.0.175
[v0.0.174]: https://github.com/agentuity/cli/compare/v0.0.173...v0.0.174
[v0.0.173]: https://github.com/agentuity/cli/compare/v0.0.172...v0.0.173
[v0.0.172]: https://github.com/agentuity/cli/compare/v0.0.171...v0.0.172
[v0.0.171]: https://github.com/agentuity/cli/compare/v0.0.170...v0.0.171
[v0.0.170]: https://github.com/agentuity/cli/compare/v0.0.169...v0.0.170
[v0.0.169]: https://github.com/agentuity/cli/compare/v0.0.168...v0.0.169
[v0.0.168]: https://github.com/agentuity/cli/compare/v0.0.167...v0.0.168
[v0.0.167]: https://github.com/agentuity/cli/compare/v0.0.166...v0.0.167
[v0.0.166]: https://github.com/agentuity/cli/compare/v0.0.165...v0.0.166
[v0.0.165]: https://github.com/agentuity/cli/compare/v0.0.164...v0.0.165
[v0.0.164]: https://github.com/agentuity/cli/compare/v0.0.163...v0.0.164
[v0.0.163]: https://github.com/agentuity/cli/compare/v0.0.162...v0.0.163
[v0.0.162]: https://github.com/agentuity/cli/compare/v0.0.161...v0.0.162
[v0.0.161]: https://github.com/agentuity/cli/compare/v0.0.160...v0.0.161
[v0.0.160]: https://github.com/agentuity/cli/compare/v0.0.159...v0.0.160
[v0.0.159]: https://github.com/agentuity/cli/compare/v0.0.158...v0.0.159
[v0.0.158]: https://github.com/agentuity/cli/compare/v0.0.157...v0.0.158
[v0.0.157]: https://github.com/agentuity/cli/compare/v0.0.156...v0.0.157
[v0.0.156]: https://github.com/agentuity/cli/compare/v0.0.155...v0.0.156
[v0.0.155]: https://github.com/agentuity/cli/compare/v0.0.154...v0.0.155
[v0.0.154]: https://github.com/agentuity/cli/compare/v0.0.153...v0.0.154
[v0.0.153]: https://github.com/agentuity/cli/compare/v0.0.152...v0.0.153
[v0.0.152]: https://github.com/agentuity/cli/compare/v0.0.151...v0.0.152
[v0.0.151]: https://github.com/agentuity/cli/compare/v0.0.150...v0.0.151
[v0.0.150]: https://github.com/agentuity/cli/compare/v0.0.149...v0.0.150
[v0.0.149]: https://github.com/agentuity/cli/compare/v0.0.148...v0.0.149
[v0.0.148]: https://github.com/agentuity/cli/compare/v0.0.147...v0.0.148
[v0.0.147]: https://github.com/agentuity/cli/compare/v0.0.146...v0.0.147
[v0.0.146]: https://github.com/agentuity/cli/compare/v0.0.145...v0.0.146
[v0.0.145]: https://github.com/agentuity/cli/compare/v0.0.144...v0.0.145
[v0.0.144]: https://github.com/agentuity/cli/compare/v0.0.143...v0.0.144
[v0.0.143]: https://github.com/agentuity/cli/compare/v0.0.142...v0.0.143
[v0.0.142]: https://github.com/agentuity/cli/compare/v0.0.141...v0.0.142
[v0.0.141]: https://github.com/agentuity/cli/compare/v0.0.140...v0.0.141
[v0.0.140]: https://github.com/agentuity/cli/compare/v0.0.139...v0.0.140
[v0.0.139]: https://github.com/agentuity/cli/compare/v0.0.138...v0.0.139
[v0.0.138]: https://github.com/agentuity/cli/compare/v0.0.137...v0.0.138
[v0.0.137]: https://github.com/agentuity/cli/compare/v0.0.136...v0.0.137
[v0.0.136]: https://github.com/agentuity/cli/compare/v0.0.135...v0.0.136
[v0.0.135]: https://github.com/agentuity/cli/compare/v0.0.134...v0.0.135
[v0.0.134]: https://github.com/agentuity/cli/compare/v0.0.133...v0.0.134
[v0.0.133]: https://github.com/agentuity/cli/compare/v0.0.132...v0.0.133
[v0.0.132]: https://github.com/agentuity/cli/compare/v0.0.131...v0.0.132
[v0.0.131]: https://github.com/agentuity/cli/compare/v0.0.130...v0.0.131
[v0.0.130]: https://github.com/agentuity/cli/compare/v0.0.129...v0.0.130
[v0.0.129]: https://github.com/agentuity/cli/compare/v0.0.128...v0.0.129
[v0.0.128]: https://github.com/agentuity/cli/compare/v0.0.127...v0.0.128
[v0.0.127]: https://github.com/agentuity/cli/compare/v0.0.126...v0.0.127
[v0.0.126]: https://github.com/agentuity/cli/compare/v0.0.125...v0.0.126
[v0.0.125]: https://github.com/agentuity/cli/compare/v0.0.124...v0.0.125
[v0.0.124]: https://github.com/agentuity/cli/compare/v0.0.123...v0.0.124
[v0.0.123]: https://github.com/agentuity/cli/compare/v0.0.122...v0.0.123
[v0.0.122]: https://github.com/agentuity/cli/compare/v0.0.121...v0.0.122
[v0.0.121]: https://github.com/agentuity/cli/compare/v0.0.120...v0.0.121
[v0.0.120]: https://github.com/agentuity/cli/compare/v0.0.119...v0.0.120
[v0.0.119]: https://github.com/agentuity/cli/compare/v0.0.118...v0.0.119
[v0.0.118]: https://github.com/agentuity/cli/compare/v0.0.117...v0.0.118
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

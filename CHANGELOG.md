# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.1.4] - 2026-08-24

### Added
- Added smart running GGUF model auto-detection and instance binding in Chat Playground.
- Added dedicated "No GGUF Server Running" guidance card with direct one-key navigation (`[2]` or `[Enter]`) to the Launch tab.
- Added interactive multi-instance model selector card when multiple GGUF servers are running.
- Added `[M]` model switcher hotkey in Chat to switch between active running GGUF server instances.
- Restricted Chat Playground strictly to GGUF model runtimes (filtering out non-chat ONNX engines).

## [2.1.3] - 2026-08-24

### Added
- Integrated Terminal Chat Playground accessible via top navigation tab `[7] Chat` and number key `7`.
- Real-time token streaming with live speed telemetry (tokens/sec), prompt token evaluation, and generated token counting over Server-Sent Events (SSE).
- 3-layer hybrid context compaction engine:
  - Layer 1: System prompt (never compacted).
  - Layer 2: Compaction Checkpoints summary stack.
  - Layer 3: Recent verbatim tail with 20% context window budget protection.
- On-demand (`[K]`) and automatic (above 85% context pressure) history compaction with full verbatim message preservation in `OriginalMessages`.
- Multiple persistent named chat sessions saved atomically under `{appDataDir}/chats/` with create (`[N]`), delete (`[D]`), and rename (`[R]`) actions.
- Dynamic Generation Parameters overlay (`[P]`) for live tuning of Temperature, Top-P, Top-K, and Context Size.
- Inline Model Launcher card when accessing chat without an active server instance.
- Clipboard copy shortcut (`[C]`) for instantaneous assistant response extraction.
- Multiline prompt editing support via `textarea` component (`Enter` to send, `Ctrl+Enter` for newlines).

### Fixed
- Fixed in-app self-updater asset matching on Windows where `darwin` release packages matched the `win` substring token, causing macOS binaries to be downloaded on Windows systems.
- Added strict mutual exclusivity and dedicated OS classifiers (`isWindowsAsset`, `isDarwinAsset`, `isLinuxAsset`) in release asset matching.
- Enhanced target executable path resolution to find and upgrade the true installed binary in system `PATH` and `%USERPROFILE%\go\bin` even when running from temporary builds.

## [2.1.2] - 2026-08-24

### Fixed
- Resolved Settings runtime inspector target scoping to eliminate cross-component updater interference.
- Implemented two-stage Enter key workflow: first Enter checks for updates, second Enter installs updates immediately.
- Normalized semantic version comparison stripping leading `v` and `b` prefixes to prevent false positive update alerts.
- Removed duplicate Theme option from Settings Runora App inspector card.

## [2.1.1] - 2026-08-24

### Added
- Automatic Assistant Library Migration from local Ollama and LM Studio installations across Windows, macOS, and Linux with zero disk weight duplication.
- Direct GitHub Release Self-Updater with standalone archive extraction, in-place binary swapping, and live download progress bars.

## [2.1.0] - 2026-08-24

### Added
- Full mouse and touchpad navigation support across the entire Bubble Tea TUI with spatial hit-test registry (`ui/mouse`).
- Clickable Persistent Global Header tabs (`[1] Models`, `[2] Launch`, `[3] Monitor`, `[4] Downloads`, `[5] Benchmarks`, `[6] Settings`).
- Model Explorer mouse interaction: single-click row selection, double-click launch transition, and context-aware mouse wheel scrolling.
- Launch Dashboard mouse interaction: 5x5 Bento grid profile selection click, double-click inference launch, and clipboard copy button.
- Server Monitor mouse interaction: live instance row selection, double-click to stream logs, and control action buttons (`[R]`, `[S]`, `[Ctrl+K]`, `[L]`).
- Downloader and Settings mouse interaction: direct input field focus clicks and component inspector navigation.
- Click-away modal dismissal for overlay dialogs (Theme Picker and help overlays).
- Cross-platform OS Keyring credentials storage (`github.com/zalando/go-keyring`) supporting Windows Credential Manager, macOS Keychain, and Linux SecretService/libsecret.
- Automatic credentials migration from plaintext `config.json` to native OS keyring with automatic token sanitization.

### Changed
- Enabled `tea.WithMouseCellMotion()` in the main Bubble Tea program loop for smooth, jitter-free cursor tracking.
- Streamlined `ProfileCreatorModel` with dedicated validation and persistence methods.

## [2.0.0] - 2026-08-23

### Added
- Persistent Global Navigation Header across all views with 1-6 numeric hotkeys (`1`: Models, `2`: Dashboard, `3`: Monitor, `4`: Downloads, `5`: Benchmark, `6`: Settings) and bidirectional `Tab` / `Shift+Tab` and `[` / `]` cycling.
- Persistent live hardware header telemetry displaying running server instance count and dynamic GPU VRAM meter.
- Bento Card layout architecture across all screens with dynamic height clamping and vertical alignment.
- 10 curated accessible themes (Dracula, Sunset, Nord, Cyberpunk, Forest, Monochrome, Solarized Light, Paper Light, and High Contrast WCAG AAA) with linear RGB gradient text and gauge rendering.
- Interactive Theme Picker modal (`[Y]`) with live swatches and descriptions.
- Non-intrusive floating toast notification system with ANSI compositing.
- Real-time Log Streamer (`[L]`) with 250ms tailing, auto-scroll, regex filter (`[/]`), pause/resume (`[Space]`), syntax highlighting, and multi-instance tab switching (`[Tab]`).
- 5-per-row Bento execution profile grid supporting up to 25 profiles with dynamic downward vertical expansion and 2D keyboard navigation (`[←/→/↑/↓]`).
- Flash Attention enabled by default across all llama.cpp server invocations and profile presets (`--flash-attn`).
- Quantized KV Cache support with `--cache-type-k` and `--cache-type-v` flags (`f16`, `q8_0`, `q4_0`, `fp8`).
- Raw Custom CLI arguments field in profiles allowing arbitrary user-supplied arguments passed directly to `llama-server`.
- Interactive 8-field Profile Creator and Editor supporting Name, Context, Threads, GPU Layers, Port, Flash Attention toggle, KV Quantization cycler, and Custom CLI Arguments.
- Unrestricted profile deletion allowing removal of any custom or built-in default profile from disk.
- Runtime version slots under `llama.cpp/versions/<tag>/` enabling side-by-side installations, instant version switching, listing, and cleanup.
- Release channel selector supporting `Stable` (vX.Y.Z releases) and `Nightly` (upstream continuous tags) with explicit backend selection (CUDA 12, CUDA 13, Vulkan, CPU, ROCm, Metal).
- Multi-GPU enumeration, total VRAM aggregation, and `TensorSplitAdvisor` GCD integer ratio calculation (e.g. 24GB + 16GB + 8GB -> `3,2,1`).
- Physical CPU core vs logical thread topology detection across Windows, Linux, and macOS.
- Apple Silicon Metal piecewise unified memory curve (67% to 92%).
- Multi-part GGUF shard auto-grouping (`model-00001-of-00004.gguf`) into consolidated single model entries with aggregate sizes.
- Multi-directory model discovery scanning primary and secondary paths (`Paths.ModelDirectories`).
- Headless CLI flags: `--list-models` (with `--json`), `--status` (with `--json`), `--data-dir <path>`, `--models <path>`, `--version`, `--reset-onboarding`.
- Comprehensive behavioral test suite hardening with boundary condition and error recovery validation.

### Changed
- Refactored model browser, launch dashboard, server monitor, downloader, benchmarks, and settings to full-screen Bento card layouts.
- Updated Settings screen left panel to clean component hierarchy: API Token, llama.cpp, ONNX Runtime, Runora App.
- Improved onboarding wizard with an 86-cell wide layout to eliminate awkward line wraps and support direct API credential configuration.
- Enforced strict zero emoji invariant across all views, headers, footers, badges, and notifications.

### Fixed
- Fixed Windows file lock race condition in atomic file writing by adding exponential backoff retry loops during high-concurrency renames.
- Fixed invisible UTF-8 BOM headers in Go source files that prevented statement coverage profiling.
- Fixed single-owner `cmd.Wait()` race conditions in process supervisor during multi-threaded instance termination.
- Fixed download queue range resumption and `.part` file cleanup on cancellation or completion.

## [1.1.1] - 2026-08-22

### Added
- Modular Theme Class architecture with CSS-variable-style semantic tokens and centralized stylesheet (global.css pattern).
- Support for llama.cpp semantic versioning releases (vX.Y.Z) with automated nightly build tag resolution via nightly-tag.txt.
- In-app configuration and environment variable support for GitHub API token (G hotkey) to increase release check limits from 60 to 5,000 req/hour.
- Direct numeric hotkeys (1, 2, 3) to switch runtime focus in the Settings view.
- Explicit inline available actions indicator for each runtime option card.

### Fixed
- Eliminated all hardcoded colors from UI components in favor of dynamic theme tokens.
- Resolved lifecycle message channel cross-contamination between runtime engines and application update checks.
- Resolved Windows CUDA asset matching for upstream continuous builds.
- Fixed column vertical separator alignment in the Preferences and Hardware info panel.
- Clean opening of Settings view without triggering automatic background downloads.
- Improved GitHub API rate limit error detection and diagnostic feedback on 403 Forbidden responses.

## [1.1.0] - 2026-08-22

### Added
- Multi-runtime abstraction architecture supporting both llama.cpp and ONNX Runtime engines.
- Unified Lifecycle and Settings screen with component focus navigation using Tab and Arrow keys.
- Universal action hotkeys for checking (C / Enter), updating (U / Space), and rolling back (R) runtime components.
- Automated backup creation and rollback restoration for ONNX Runtime library installations.
- Magic-byte archive format sniffing supporting ZIP, Nupkg, Tar.gz, and TGZ packages without relying on file extensions.
- Support for searching and downloading .onnx model files directly from Hugging Face repositories.
- Automatic configuration directory migration from legacy paths to the runora application directory.
- Dynamic port allocation and socket collision mitigation for concurrent multi-model server instances.
- Onboarding walkthrough tour and visual gradient theme switcher.

### Changed
- Rebranded project, Go module path, CLI command, and configuration namespace from llama-manager/llmgr to Runora/runora.
- Streamlined settings footer keybindings into an intuitive unified layout.
- Updated Hugging Face search filters to query both GGUF and ONNX format repositories.

### Fixed
- Fixed ONNX runtime installation error caused by unsupported .archive temporary file extensions.
- Resolved race conditions in download task queue scheduling during concurrent model operations.
- Resolved socket race condition during rapid sequential server deployments on identical ports.

## [1.0.0] - 2026-08-20

### Added
- Core TUI model browser with real-time fuzzy search and model metadata inspection.
- Hardware discovery engine detecting CPU, RAM, and GPU capabilities with VRAM estimation.
- Custom launch profiles supporting custom context lengths, GPU layer offloading, and thread counts.
- Integrated benchmark suite measuring token generation throughput and memory footprints.
- Server monitoring dashboard displaying instance uptime, resident memory RSS, and status.
- Hugging Face repository model downloader with pause, resume, and cancellation support.

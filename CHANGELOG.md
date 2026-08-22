# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.1.1] - 2026-08-22

### Added
- Support for llama.cpp semantic versioning releases (vX.Y.Z) with automated nightly build tag resolution via nightly-tag.txt.

### Fixed
- Resolved lifecycle message channel cross-contamination between runtime engines and application update checks.
- Resolved Windows CUDA asset matching for upstream continuous builds.

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

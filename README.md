# Runora

A terminal-based manager, launcher, and monitoring dashboard for local large language models. Runora handles recursive GGUF/ONNX model discovery, multi-part shard aggregation, exact hardware suitability estimation, 5x5 execution profile grids, Flash Attention acceleration, quantized KV cache configuration, multi-instance server supervision, automated runtime version slot management, and high-speed model downloads.

---

## Key Features

- **Global Number-Row Navigation & Keymap**:
  - Instantaneous top-level routing using number keys `1` through `6` across all views:
    - `[1] Models`: Bento-card model explorer and metadata inspection deck.
    - `[2] Dashboard`: Dual-column launch dashboard, 5x5 profile grid, and CLI preview.
    - `[3] Monitor`: Multi-instance server supervisor, live `/slots` context gauges, and throughput metrics.
    - `[4] Downloads`: Direct URL / Hugging Face download manager with queue resumption.
    - `[5] Benchmark`: Decoupled prompt latency (TTFT) and generation throughput dashboard.
    - `[6] Settings`: Multi-runtime version slots, release channels, and API token manager.
  - Global cycling with `Tab` / `Shift+Tab` and `[` / `]`.
  - Persistent hardware telemetry header displaying live active server counts and GPU VRAM utilization meters.

- **Bento Card TUI Design & 10 Built-In Themes**:
  - Modular Bento surface card architecture across all screens with responsive height clamping.
  - 10 curated color themes including Dracula, Sunset, Nord, Cyberpunk, Forest, Monochrome, Solarized Light, Paper Light, and High Contrast (WCAG AAA).
  - Interactive Theme Picker (`[Y]`) with live preview and description swatches.
  - Non-intrusive floating toast notifications with ANSI compositing.
  - Real-time live log streamer (`[L]`) with 250ms tailing, auto-scroll, regex search filtering (`[/]`), stream pause/resume (`[Space]`), syntax highlighting, and multi-instance tab switching.

- **5x5 Profile Grid & Deep Customization**:
  - 5-per-row Bento grid layout (supporting up to 25 profiles) with dynamic vertical expansion that pushes subsequent content downward without phantom line gaps.
  - 2D grid keyboard navigation (`[←/→]` for horizontal movement, `[↑/↓]` for vertical row jumps).
  - Flash Attention enabled by default across all llama.cpp invocations (`--flash-attn`).
  - Quantized KV Cache support (`--cache-type-k` and `--cache-type-v` supporting `f16`, `q8_0`, `q4_0`, `fp8`).
  - Direct raw CLI arguments field allowing custom flags passed directly to `llama-server`.
  - Interactive 8-field Profile Creator and Editor (`[P]` / `[E]`).
  - Unrestricted profile deletion (`[D]`) allowing removal of any custom or built-in default profile.

- **Multi-Runtime Architecture & Version Slot Management**:
  - Multi-runtime runner abstraction with deterministic routing:
    - **llama.cpp**: Native runner for `.gguf` text generation and embedding models.
    - **ONNX Runtime**: Native execution wrapper for `.onnx` models.
  - Version slot isolation under `llama.cpp/versions/<tag>/` with active version switching, side-by-side installations, and clean removal.
  - Release channel switching between `Stable` (vX.Y.Z releases) and `Nightly` (upstream commit tags).
  - Explicit backend accelerator selection: CUDA 12, CUDA 13, Vulkan, CPU, ROCm, Metal.
  - Automated asset matching and dependency extraction (including Windows CUDA runtime DLLs).

- **Hardware Intelligence & Multi-GPU Tensor Splitting**:
  - Native host environment detection covering CPU physical cores vs logical threads, RAM, and GPU accelerators.
  - Multi-GPU enumeration, total VRAM aggregation, and `TensorSplitAdvisor` integer ratio calculation (e.g. 24GB + 16GB + 8GB -> `3,2,1`).
  - Mathematical Quantized KV Cache memory estimation (`FP16`, `Q8_0`, `Q4_0`, `FP8`) and exact integer GPU layer offloading calculator.
  - Apple Silicon Metal piecewise unified memory curve (67% to 92%).

- **GGUF Sharding & Multi-Directory Model Discovery**:
  - Automatic multi-part GGUF shard consolidation (`model-00001-of-00004.gguf`) into single entries with aggregate size and shard count indicators.
  - Multi-directory recursive model discovery supporting primary and secondary storage paths (`Paths.ModelDirectories`).
  - Safe GGUF metadata parsing protected against oversized strings, recursion loops, and corrupted headers.

- **Download Queue with HTTP Range Resumption**:
  - Direct URL and Hugging Face repository search and download queue.
  - HTTP Range header resumption with `.part` file caching and graceful fallback.

- **Headless CLI Scripting**:
  - Headless execution flags for terminal pipelines and automated environments:
    - `runora --list-models`: List discovered models (supports `--json`).
    - `runora --status`: Print active server instances and telemetry (supports `--json`).
    - `runora --data-dir <path>`: Override the default application data directory.
    - `runora --models <path>`: Override the primary models search directory.

---

## Requirements

- **Go 1.22** or later (for compiling from source).
- **Supported Operating Systems**: Windows (10/11), Linux, and macOS (Apple Silicon and Intel).
- **Accelerators**: NVIDIA CUDA Toolkit (11.x, 12.x, 13.x), Vulkan SDK, AMD ROCm, Apple Metal, or CPU fallback.

---

## Installation

### Compile from Source via Go

Ensure Go is installed and present in your system's `PATH`:

```bash
go install github.com/BIJJUDAMA/runora/cmd/runora@latest
```

The compiled binary will be placed in your Go bin directory (typically `$HOME/go/bin` on Linux/macOS or `%USERPROFILE%\go\bin` on Windows).

### Pre-Built GitHub Releases

Download the latest pre-compiled archive for your platform from the [GitHub Releases](https://github.com/BIJJUDAMA/runora/releases) page.

---

## Usage

Start the interactive terminal user interface:

```bash
runora
```

### CLI Command Options

```text
Usage: runora [flags]

Flags:
  --version            Print Runora version and exit
  --list-models        List all discovered models and exit
  --status             Query running server instances and exit
  --json               Format command output as JSON (used with --list-models or --status)
  --data-dir <path>    Custom application data directory
  --models <path>      Custom primary models directory
  --reset-onboarding   Reset the interactive onboarding wizard
```

---

## Keyboard Shortcuts

### Global Navigation

- `1` / `2` / `3` / `4` / `5` / `6`: Jump directly to screen (1: Models, 2: Dashboard, 3: Monitor, 4: Downloads, 5: Benchmark, 6: Settings).
- `Tab` / `Shift+Tab` or `]` / `[`: Cycle forward / backward through the 6 main screens.
- `Y`: Open interactive Theme Picker modal.
- `L`: Open real-time Log Streamer.
- `?`: Toggle help and key binding overlay.
- `Q` / `Ctrl+C`: Stop all running instances and exit.

### [1] Models Screen

- `↑` / `↓` or `k` / `j`: Move selection in the model list.
- `Enter` / `Space`: Open Launch Dashboard for selected model.
- `F`: Toggle favorite status.
- `B`: Launch benchmark run for selected model.
- `E`: Cycle task type (TEXT_GENERATION, EMBEDDING, VISION, etc.).
- `/`: Activate search filter.

### [2] Launch Dashboard

- `←` / `→` or `h` / `l`: Move profile selection horizontally.
- `↑` / `↓` or `k` / `j`: Move profile selection vertically across grid rows (5 per row).
- `Enter`: Launch server with selected profile.
- `P`: Create new custom launch profile.
- `E`: Edit currently selected profile.
- `N`: Duplicate currently selected profile.
- `D`: Delete currently selected profile.
- `C`: Copy generated CLI command string to clipboard.
- `Esc`: Return to model explorer.

### [3] Server Monitor

- `↑` / `↓` or `k` / `j`: Select active server instance.
- `R`: Restart selected server instance.
- `S`: Stop selected server instance.
- `Ctrl+K`: Terminate all active server instances.
- `L`: Stream live logs for selected server instance.

### [4] Downloads Screen

- `Tab` / `Shift+Tab`: Switch focus between URL input, curated models, and download queue.
- `Enter`: Queue model download.
- `P`: Pause active download task.
- `R`: Resume paused download task.
- `X`: Cancel active download and remove temporary files.
- `C`: Clear completed and failed tasks from queue.

### [5] Benchmark Screen

- `B`: Start a new benchmark run on the selected model.
- `↑` / `↓`: Scroll benchmark history and throughput telemetry.

### [6] Settings Screen

- `1` / `2` / `3` / `4` / `5`: Select component (1: API Tokens, 2: llama.cpp, 3: ONNX Runtime, 4: Runora App, 5: Model Sources).
- `C` / `Enter`: Check for runtime/application updates.
- `U` / `Space`: Download and install update slot.
- `R`: Roll back selected component to backup or rescan external model sources.
- `Space`: Toggle enabled status for selected external model source.
- `E`: Edit API tokens (GitHub / Hugging Face).
- `S`: Save updated API tokens.

---

## Mouse and Touchpad Navigation

Runora supports first-class mouse and touchpad interaction alongside the keyboard workflow:

- **Top Navigation Tabs**: Click any header tab (`[1] Models`, `[2] Launch`, `[3] Monitor`, `[4] Downloads`, `[5] Benchmarks`, `[6] Settings`) to switch views directly.
- **Model Explorer**: Click any model row to select; double-click to immediately open the Launch Dashboard; use the scroll wheel to browse large model libraries.
- **Launch Dashboard**: Click any profile card in the 5x5 Bento grid to select; double-click to immediately launch the inference server; click `[C] Copy Launch Command` to copy to clipboard.
- **Server Monitor**: Click any active server instance to inspect runtime metrics; double-click to stream logs; click action buttons (`[R]`, `[S]`, `[Ctrl+K]`, `[L]`).
- **Downloader**: Click URL or destination filename input fields to focus and type; click queue tasks to inspect progress.
- **Settings & Inspector**: Click component items in the left panel to inspect details; click input fields to edit API tokens.
- **Theme Picker & Modals**: Click themes to preview; double-click to apply and dismiss; click anywhere outside the modal dialog to dismiss.

---

## Data Directory Layout

Runora organizes all local state, binaries, and configurations within a clean, isolated data directory (`%LOCALAPPDATA%\runora` or `%APPDATA%\runora` on Windows, `~/.local/share/runora` on Linux, `~/Library/Application Support/runora` on macOS):

```text
runora/
├── config.json              # Application preferences, favorites, and directory paths
├── models/                  # Primary recursive model storage directory
├── profiles/                # JSON profile configurations (5x5 Bento grid)
├── benchmarks/              # Benchmark history database (history.json)
├── downloads/               # Active download workspace and .part files
├── cache/                   # GGUF metadata cache and runtime logs
└── llama.cpp/               # Active llama.cpp runtime binary and CUDA DLLs
    └── versions/            # Version slots (e.g. b4850/, v0.2.0/)
```

---

## License

This project is licensed under the Apache 2.0 License. See [LICENSE](LICENSE) for details.

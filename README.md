# Runora

A terminal-based manager and launcher for local large language models. It handles model discovery, hardware suitability estimation, custom launch profiles, concurrent server execution, model downloads (from URLs or Hugging Face repositories), and runtime lifecycle management.

---

## Key Features

- **Multi-Runtime Architecture**: Abstracted runner layer with deterministic routing of models:
  - **llama.cpp**: Out-of-the-box support for `.gguf` text generation and embedding models.
  - **ONNX Runtime**: Native execution wrapper for `.onnx` models.
- **Dynamic Hardware & CUDA Detection**: Dynamic host environment interrogation (operating system, CPU threads/model, RAM, and GPU accelerator).
  - Automatically identifies **CUDA**, **Metal**, **ROCm**, **Vulkan**, or **CPU-only** backends.
  - Dynamically extracts system CUDA toolkit versions (major versions 11, 12, 13) using environment variables, command queries (`nvcc`), or `nvidia-smi` parser blocks to download the optimal prebuilt binary.
- **Automated Pre-Built Runtime Installers**:
  - **llama.cpp Updater**: Downloads, validates, and extracts pre-built binary packages and Windows CUDA runtime DLLs matching the system's exact CUDA version.
  - **ONNX Runtime Downloader**: Automatically checks the official `microsoft/onnxruntime` repository, matches the host architecture/CUDA backend, extracts only the required shared libraries (`onnxruntime.dll`, `libonnxruntime.so`, or `libonnxruntime.dylib`), and validates the installation.
- **Concurrent Model Execution**: Launch multiple model instances concurrently on separate ports:
  - Dynamic **port auto-fallback**: If a model is launched on a port currently occupied by another active server, Runora automatically increments the port (e.g. `50506`, `50507`) to run both side-by-side.
  - Safe port releasing delays (250ms cooldown) to avoid socket race conditions when restarting/re-deploying a model.
- **Interactive Model Download Manager**:
  - Queue direct URLs or resolve entire **Hugging Face repositories** dynamically.
  - Interactively browse and select specific GGUF/model files within Hugging Face repos to queue for download.
  - Real-time download speeds, percentage indicators, and individual/completed task cleaning.
- **Task Type Cycling**: Cycle model task capabilities directly from the TUI (`[E]` key) to transition between text generation, embeddings, reranking, speech, image generation, vision, and multimodal types.
- **Beautiful TUI Aesthetics**: Custom built-in gradient themes (Dracula, Sunset, Nord, Cyberpunk, Forest, Monochrome) cycleable directly via keyboard hotkeys.

---

## Requirements

- **Go 1.26** or later (for compiling/building from source).
- **Supported Platforms**: Windows, Linux, and macOS (with Apple Silicon unified memory detection).
- **GPU Toolkits**: NVIDIA CUDA Toolkit (if running CUDA acceleration; fallback to CPU is automatic if missing).

---

## Installation

Ensure Go is installed and present in your system's `PATH`. Run the following command to download and compile the application:

```bash
go install github.com/BIJJUDAMA/runora/cmd/runora@latest
```

The compiled binary will be placed in your Go bin directory (typically `$HOME/go/bin` on Unix or `%USERPROFILE%\go\bin` on Windows).

To temporarily add it to your path:

### Windows (PowerShell)

```powershell
$env:PATH += ";$env:USERPROFILE\go\bin"
```

### Linux / macOS

```bash
export PATH="$HOME/go/bin:$PATH"
```

---

## Usage

Start the application by running:

```bash
runora
```

### Command Flags

- `--version`: Print the installed version of Runora and exit.
- `--reset-onboarding`: Re-run the interactive onboarding tour on the next startup.

---

## Keyboard Navigation Guide

Runora is fully keyboard-driven. Below are the key mappings available across the principal TUI screens:

### Main Browser Screen

- `↑` / `↓` or `k` / `j`: Move selection in the sidebar.
- `Tab` / `Shift+Tab`: Toggle focus between the sidebar list and model details pane.
- `/`: Activate search filter to quickly locate models by name, architecture, or filepath.
- `Space` / `Enter`: Open the launch dashboard for the selected model.
- `s` / `S`: Stop the currently selected model instance.
- `Ctrl+S`: Terminate all active local servers.
- `e` / `E`: Cycle task type for the selected model (TEXT_GENERATION, EMBEDDING, etc.).
- `d` / `D`: Jump to the Download Manager screen.
- `u` / `U`: Open the Settings / Lifecycle screen.
- `m` / `M`: Open the Server Monitor screen (physical memory RSS tracking, request counters).
- `v` / `V`: Open the Performance Benchmark History dashboard.
- `b` / `B`: Run speed benchmark on the selected model.
- `q` / `Ctrl+C`: Stop all running servers and quit the application.

### Settings / Lifecycle Screen

- `1` / `2` / `3` or `Tab` / `Shift+Tab` / `↑` / `↓`: Switch focus between components (1: llama.cpp, 2: ONNX Runtime, 3: Runora App).
- `C` / `Enter`: Check for updates on the selected runtime or application.
- `U` / `Space`: Download and install updates for the selected runtime or application.
- `R`: Roll back the selected runtime to the previous backup version.
- `Y`: Cycle visual theme.
- `T`: Edit Hugging Face API token (`HF_TOKEN`) for gated models.
- `G`: Edit GitHub Personal Access Token (`GITHUB_TOKEN`) to expand release check rate limits.
- `N`: Reset the interactive onboarding tour.
- `Esc`: Return to the main model browser.

---

## Data Directory Layout

All application files are stored inside a platform-conventional data folder (`%APPDATA%\runora` on Windows). The folder structure is organized as follows:

```text
runora/
├── config.json         # Persistent user preferences, favorites, and task overrides
├── models/             # Root folder scanned recursively for GGUF/ONNX models
├── llama.cpp/          # Local directory containing llama-server and CUDA DLLs
├── llama.cpp.backup/   # Rollback backup folder created before updates
├── onnxruntime/        # Dedicated folder containing ONNX shared libraries
├── downloads/          # Temporary directory for active asset downloads
└── profiles/           # Custom launch profile configuration files (.json)
```

---

## Configuration

Custom launch profiles are defined in `.json` files inside the `profiles/` directory. Each profile specifies hardware offloading, threads, context sizes, and ports:

```json
{
  "name": "Custom GPU Profile",
  "context": 4096,
  "threads": 4,
  "gpu_layers": 999,
  "batch_size": 512,
  "host": "127.0.0.1",
  "port": 50505
}
```

---

## License

This project is licensed under the Apache 2.0 License. See [LICENSE](LICENSE) for details.

# pathsize

A fast, single-binary terminal UI (TUI) disk-usage browser. Scan a directory,
see what eats your space, expand folders interactively. Works on **Windows,
Linux, and macOS**.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Install

### `go install` (requires Go 1.25+)

```sh
go install github.com/nicolas-camacho/pathsize@latest
```

The binary lands in `$(go env GOBIN)` (or `$(go env GOPATH)/bin`). Make sure
that directory is on your `PATH`.

### Prebuilt binaries

Download the archive for your OS/arch from the
[Releases](https://github.com/nicolas-camacho/pathsize/releases) page, extract
it, and put the `pathsize` (or `pathsize.exe`) binary on your `PATH`.

| OS      | Archive            | Arch          |
| ------- | ------------------ | ------------- |
| Linux   | `.tar.gz`          | amd64, arm64  |
| macOS   | `.tar.gz`          | amd64, arm64  |
| Windows | `.zip`             | amd64, arm64  |

Verify downloads against `checksums.txt`.

## Usage

```sh
pathsize [flags] [path] [depth]
```

- `path` — directory to scan (default `.`, the current directory)
- `depth` — how many levels to expand, integer `>= 1` (default `2`)

### Flags

| Flag             | Description                                                  |
| ---------------- | ----------------------------------------------------------- |
| `-v`, `--version`| print version and exit                                      |
| `-h`, `--help`   | print help and exit                                         |
| `--no-tui`       | print results as plain text instead of the TUI              |
| `--json`         | print results as JSON (implies `--no-tui`)                  |
| `--min-size S`   | hide entries smaller than `S` (e.g. `10MB`, `500K`, `1.5G`) |

> Flags must come before the positional `path`/`depth` arguments.

The `--no-tui` and `--json` modes run without a terminal UI, so they work in
pipes, scripts, and non-interactive shells. Size units accept `B`, `K`/`KB`,
`M`/`MB`, `G`/`GB`, `T`/`TB` (a bare number is bytes).

Examples:

```sh
pathsize                       # scan current dir, 2 levels (TUI)
pathsize /var/log              # scan /var/log, 2 levels
pathsize . 4                   # scan current dir, 4 levels deep
pathsize --min-size 100MB /    # TUI, hide entries under 100 MB
pathsize --no-tui . 1          # plain-text listing, one level
pathsize --json --min-size 1G . 3 > sizes.json   # JSON report
pathsize -v                    # print version
pathsize -h                    # print help
```

### Keys

| Key                     | Action            |
| ----------------------- | ----------------- |
| `↑`/`k`, `↓`/`j`        | move cursor       |
| `pgup`/`ctrl+b`, `pgdn`/`ctrl+f` | page up/down |
| `g` / `G`               | top / bottom      |
| `enter` / `space`       | expand / collapse |
| `q` / `esc` / `ctrl+c`  | quit              |

> Note: directory sizes are always the **full recursive** total, regardless of
> the `depth` you expand to. `depth` only controls how deep the tree is built
> for browsing.

## Build from source

Requires Go 1.25+.

```sh
git clone https://github.com/nicolas-camacho/pathsize.git
cd pathsize
go build -o pathsize .       # local binary
```

### Cross-compile every platform

```sh
make dist          # Linux/macOS -> dist/
pwsh ./build.ps1   # Windows     -> dist/
```

Both produce binaries for linux/darwin/windows on amd64 + arm64 in `dist/`.

## Releasing

Tag and push; GitHub Actions runs [GoReleaser](https://goreleaser.com) to build
all platforms, create archives + checksums, and publish a GitHub Release.

```sh
git tag v1.0.0
git push origin v1.0.0
```

## License

[MIT](LICENSE)

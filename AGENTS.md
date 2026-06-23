# AGENTS.md

## What this is
`pathsize`: a single-binary terminal UI (Bubble Tea) disk-usage browser. All app code lives in `main.go` (~360 lines, package `main`). Module path is `github.com/nicolas-camacho/pathsize`. There are no unit tests; CI only builds + vets.

## Commands
- Build: `go build -o pathsize .` (or `make build`)
- Run: `go run . [path] [depth]` — `path` defaults to `.`, `depth` (levels to expand) defaults to `2`, must be int >= 1
- Format / vet before committing: `gofmt -w main.go` and `go vet ./...` (or `make all`)
- Cross-compile all platforms: `make dist` (Linux/macOS) or `pwsh ./build.ps1` (Windows) → `dist/`

## Gotchas
- Built binaries are git-ignored (`.gitignore`): `dist/`, `pathsize`, `*.exe`. Do NOT commit binaries — releases ship via GoReleaser, not the repo.
- This is an interactive full-screen TUI (`tea.WithAltScreen`). It will not run usefully in a non-TTY agent shell; verify changes by `go build` / `go vet`, not by running it. Exception: `pathsize -v|-h` print and exit (no TUI), safe to run.
- Requires Go 1.25.0 (see `go.mod`).
- `version` var in `main.go` is injected at build time via `-ldflags "-X main.version=..."`; defaults to `"dev"`. Build scripts derive it from `git describe`.

## Release flow
- Push a `vX.Y.Z` tag → `.github/workflows/release.yml` runs GoReleaser (`.goreleaser.yaml`) → builds linux/darwin/windows × amd64/arm64, archives + `checksums.txt`, publishes a GitHub Release.
- `.github/workflows/ci.yml` builds + vets on push/PR across the 3 OSes.

## Architecture notes (not obvious from one file)
- `scan` walks `maxLevel` directory levels deep (user-supplied `depth`, passed via `model.maxLevel` into `Init`), but `dirSize` always computes the full recursive size of every directory regardless of depth.
- Nodes are sorted largest-first by size at every level in `scan`.
- The Bubble Tea `model` keeps the full tree in `roots`; `flatten()` recomputes the visible list each frame from `expanded` flags. Scrolling math (`offset`, `clampScroll`, `listHeight`) assumes 4 rows of chrome around the list.
- View indent scales with `n.level` (`strings.Repeat("   ", n.level-1)`), so arbitrary depth renders correctly.
- Unreadable files/dirs are silently skipped (errors swallowed in `dirSize` and `scan`).

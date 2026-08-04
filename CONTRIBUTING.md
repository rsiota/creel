# Contributing to creel

Thanks for your interest in improving creel! This guide covers building,
testing, and the conventions that keep the project consistent. Bug reports and
feature requests are welcome too — see the [issue templates](.github/ISSUE_TEMPLATE).

## Prerequisites

- **Go 1.26+** (check `go.mod`)
- A terminal with Unicode support

## Build & run

```sh
git clone https://github.com/rsiota/creel.git
cd creel
go build -o creel ./cmd/creel/
./creel -database demo/creel-demo.db     # explore the bundled sample database
```

## Tests

Run the full suite:

```sh
go test ./...
```

A few highlights worth knowing about:

- **`internal/ui/registry_test.go`** — `TestRegistryConsistency` and
  `TestKeybindingsMatchDispatch` are drift-detection tests: they ensure the help
  overlay and command palette stay in sync with the keys actually wired into
  dispatch. If you add or change a keybinding, these will tell you what to
  update.
- **`internal/db/*_test.go`** — driver behaviour tests, one set per database.

## Conventions

### Commits

This project follows [Conventional Commits](https://www.conventionalcommits.org):
`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `ci:`, `style:`, … Keep the
subject line short and imperative. Scope is optional but used for UI work
(`feat(ui): …`).

### Code

- Match the surrounding style — `gofmt`/`goimports` is assumed.
- New behaviour needs a test. UI changes should land with a test in
  `internal/ui/`.
- Pure Go only — no CGO. SQLite uses `modernc.org/sqlite` so the project
  cross-compiles cleanly; please don't introduce a CGO dependency.

## Extending creel

### Adding a keybinding

1. Register it in **`internal/ui/registry.go`** — this is the single source of
   truth for both the `?` help overlay and the `Ctrl+P` palette.
2. Wire the key into dispatch.
3. The drift-detection tests above will fail until the help/palette reflect the
   new binding, which is the prompt to update them.
4. If the key is user-facing, add it to [docs/keybindings.md](docs/keybindings.md).

### Adding a database driver

1. Implement the `DB` interface in **`internal/db/db.go`** in a new file under
   `internal/db/`.
2. Register the driver in the connection layer and the CLI `-driver` flag.
3. Add driver tests mirroring the existing `sqlite`/`mysql`/`postgres` ones.

## Pull requests

1. Fork and branch from **`main`**.
2. Keep PRs focused — one logical change each. If a change spans docs and code,
   that's fine, but say so in the description.
3. Make sure `go build ./...` and `go test ./...` pass locally; CI runs the same
   on every push (`.github/workflows/ci.yml`).
4. Reference any issue the PR closes (`Closes #123`).
5. For user-facing changes, update the relevant doc under `docs/` and the
   README if needed, and add an entry under **[Unreleased]** in
   [CHANGELOG.md](CHANGELOG.md).

## Reporting issues

- **Bugs / features** → [issue templates](.github/ISSUE_TEMPLATE). Include the
  output of `creel -version` and steps to reproduce.
- **Security** → see [SECURITY.md](SECURITY.md); do **not** open a public issue.

By participating you agree to abide by the [Code of Conduct](CODE_OF_CONDUCT.md).

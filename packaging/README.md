# Packaging (Scoop + AUR)

Thin wrappers around [GitHub Releases](https://github.com/rsiota/creel/releases)
produced by GoReleaser. Same pattern as the Homebrew tap
(`rsiota/homebrew-creel`): bump URL + checksum when a tag ships.

| Channel | Install | Source of truth here |
| --- | --- | --- |
| **Scoop** (Windows) | `scoop bucket add creel https://github.com/rsiota/scoop-creel` then `scoop install creel` | `scoop/creel.json` → published to [rsiota/scoop-creel](https://github.com/rsiota/scoop-creel) |
| **AUR** (Arch) | `yay -S creel-bin` (once published) | `aur/creel-bin/` → push to `ssh://aur@aur.archlinux.org/creel-bin.git` |

Nix is deferred. GoReleaser’s Scoop publisher is Pro-only, so updates are
scripted instead.

## After each `v*` release

```sh
./scripts/update-packaging.sh v0.5.0   # or omit the tag for latest
# commit packaging/ in this repo

# Scoop bucket
cp packaging/scoop/creel.json /path/to/scoop-creel/creel.json
# commit + push rsiota/scoop-creel

# AUR (first time: see below)
cp packaging/aur/creel-bin/PKGBUILD packaging/aur/creel-bin/.SRCINFO /path/to/creel-bin/
# git commit + git push in the AUR checkout
```

Or with a local Scoop bucket clone:

```sh
SCOOP_BUCKET_DIR=~/code/scoop-creel ./scripts/update-packaging.sh v0.5.0 --push-scoop
```

Also bump the Homebrew tap formula as today.

## First-time AUR publish

1. Create an [AUR account](https://aur.archlinux.org/) and upload your SSH key.
2. On a machine with `makepkg`:

   ```sh
   git clone ssh://aur@aur.archlinux.org/creel-bin.git
   cp packaging/aur/creel-bin/PKGBUILD packaging/aur/creel-bin/.SRCINFO creel-bin/
   cd creel-bin
   # optional: makepkg -si   # smoke-test on Arch/chroot
   git add PKGBUILD .SRCINFO
   git commit -m "creel-bin 0.5.0-1"
   git push
   ```

3. Until that push exists, Arch users can still install from this tree with
   `makepkg -si` inside `packaging/aur/creel-bin`.

## Scoop bucket repo

The public bucket is a tiny repo whose root contains only `creel.json` (and a
short README). Keep it in sync with `packaging/scoop/creel.json` after every
release so `scoop update` picks up new versions.

# Third-Party Theme Attribution

creel ships a large catalog of color themes auto-derived from the
[iTerm2-Color-Schemes](https://github.com/mbadolato/iTerm2-Color-Schemes)
project. These are the themes listed under "generated" in the theme picker
(`g c`), distinct from creel's hand-tuned curated themes (Tokyo Night, Gruvbox,
Nord, Catppuccin, Light).

## Source

- **Upstream repo:** https://github.com/mbadolato/iTerm2-Color-Schemes
- **Format used:** Windows Terminal JSON (`windowsterminal/*.json`)
- **Snapshot:** the exact scheme JSONs used for generation are committed under
  `cmd/genthemes/schemes/` so the catalog is reproducible without network
  access. To refresh from upstream, replace that directory's contents with a
  fresh copy of `windowsterminal/` and run `go run ./cmd/genthemes`.

## Derivation

Each scheme's 16 ANSI colors + background/foreground are mapped onto creel's
19 semantic color slots by `cmd/genthemes` (see its doc comment for the
mapping). This is a mechanical derivation, not a port: the upstream color
values are reused, but the semantic assignment (e.g. which color becomes the
"primary" or "border") is creel's. Schemes whose foreground/background contrast
fails WCAG AA (< 4.5:1) are skipped at generation time.

## Licenses

The iTerm2-Color-Schemes collection is MIT-licensed. Individual themes are
contributed by many authors under their own licenses (overwhelmingly MIT, with
some per--theme exceptions). The original color values and theme names retain
their authors' licenses and attribution; see the upstream repository for
per-theme authorship and license details.

creel's derived palettes (`internal/ui/themes_generated.go`) are generated code
that reuses those color values; the generator's output header records the
upstream source.

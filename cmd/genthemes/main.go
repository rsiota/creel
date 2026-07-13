// Command genthemes derives gsql color palettes from iTerm2-Color-Schemes
// (Windows Terminal JSON format) and emits a Go source file that registers
// them as generatedThemes in package ui.
//
// Usage:
//
//	go run ./cmd/genthemes [-schemes DIR] [-out FILE]
//
// The schemes directory should contain Windows Terminal *.json color-scheme
// files (as found in the iTerm2-Color-Schemes repo's windowsterminal/ folder).
// The output is formatted Go source suitable for committing.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// scheme is the subset of the Windows Terminal color-scheme JSON we consume.
type scheme struct {
	Name                string `json:"name"`
	Background          string `json:"background"`
	Foreground          string `json:"foreground"`
	SelectionBackground string `json:"selectionBackground"`
	Black               string `json:"black"`
	Red                 string `json:"red"`
	Green               string `json:"green"`
	Yellow              string `json:"yellow"`
	Blue                string `json:"blue"`
	Purple              string `json:"purple"`
	Cyan                string `json:"cyan"`
	White               string `json:"white"`
	BrightBlack         string `json:"brightBlack"`
	BrightRed           string `json:"brightRed"`
	BrightGreen         string `json:"brightGreen"`
	BrightYellow        string `json:"brightYellow"`
	BrightBlue          string `json:"brightBlue"`
	BrightPurple        string `json:"brightPurple"`
	BrightCyan          string `json:"brightCyan"`
	BrightWhite         string `json:"brightWhite"`
}

// palette holds the 19 semantic color slots as hex strings, mirroring
// ui.colorPalette (same field names and order) so the emitter can write them
// directly into a colorPalette literal.
type palette struct {
	primary, accent, success, mark, search, visual, cursorRow, edit, warn, err,
	muted, label, border, borderUnfocused, bg, stripe, fg, highlight, statusBarBg string
}

// genEntry pairs a normalized theme key with its original display name and
// derived palette.
type genEntry struct {
	key     string
	display string
	pal     palette
}

type rgb struct{ r, g, b int }

func parseHex(s string) rgb {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return rgb{}
	}
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return rgb{}
	}
	return rgb{int(n>>16) & 0xff, int(n>>8) & 0xff, int(n) & 0xff}
}

func (c rgb) hex() string { return fmt.Sprintf("#%02x%02x%02x", c.r, c.g, c.b) }

// mix linearly interpolates between a and b by t (0=a, 1=b). Mixing bg toward
// fg by a small fraction lightens a dark bg and darkens a light bg, so the
// same formula yields appropriate tints for both light and dark themes.
func mix(a, b rgb, t float64) rgb {
	lerp := func(x, y int) int { return int(math.Round(float64(x)*(1-t) + float64(y)*t)) }
	return rgb{lerp(a.r, b.r), lerp(a.g, b.g), lerp(a.b, b.b)}
}

// relLum is the WCAG relative luminance of an sRGB color.
func relLum(c rgb) float64 {
	ch := func(v int) float64 {
		f := float64(v) / 255
		if f <= 0.03928 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c.r) + 0.7152*ch(c.g) + 0.0722*ch(c.b)
}

func contrast(a, b rgb) float64 {
	la, lb := relLum(a), relLum(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// derive maps a terminal color scheme onto gsql's 19 semantic slots. Accent
// slots come straight from the ANSI colors; tint slots (borders, cursor row,
// status bar, search/visual) are synthesized by interpolating between bg and
// fg. The unfocused border needs a light/dark branch (light themes fade toward
// bg, dark themes lift toward fg).
func derive(s scheme) palette {
	bg := parseHex(s.Background)
	fg := parseHex(s.Foreground)
	light := relLum(bg) > 0.4

	sel := parseHex(s.SelectionBackground)
	if s.SelectionBackground == "" {
		sel = mix(bg, fg, 0.16)
	}

	brightBlack := parseHex(s.BrightBlack)
	border := mix(bg, fg, 0.22)
	borderUnfocused := mix(border, bg, 0.30)
	if !light {
		borderUnfocused = mix(border, fg, 0.15)
	}

	edit := parseHex(s.BrightYellow)
	if edit == (rgb{}) {
		edit = parseHex(s.Yellow)
	}

	return palette{
		primary:         parseHex(s.Blue).hex(),
		accent:          parseHex(s.Purple).hex(),
		success:         parseHex(s.Green).hex(),
		mark:            parseHex(s.Cyan).hex(),
		search:          mix(bg, fg, 0.18).hex(),
		visual:          sel.hex(),
		cursorRow:       mix(bg, fg, 0.12).hex(),
		edit:            edit.hex(),
		warn:            parseHex(s.Yellow).hex(),
		err:             parseHex(s.Red).hex(),
		muted:           brightBlack.hex(),
		label:           mix(brightBlack, fg, 0.5).hex(),
		border:          border.hex(),
		borderUnfocused: borderUnfocused.hex(),
		bg:              bg.hex(),
		stripe:          mix(bg, fg, 0.06).hex(),
		fg:              fg.hex(),
		highlight:       mix(bg, fg, 0.12).hex(),
		statusBarBg:     mix(bg, fg, 0.10).hex(),
	}
}

// normalize turns a theme name into a config key: lowercased, with spaces
// and camelCase boundaries become hyphens (e.g. "Atom One Dark" ->
// "atom-one-dark", "TokyoNight Storm" -> "tokyo-night-storm"). The original
// (display) name is preserved separately for the picker.
func normalize(name string) string {
	var b strings.Builder
	prevLower := false
	for _, r := range name {
		if r == ' ' {
			b.WriteRune('-')
			prevLower = false
			continue
		}
		if unicode.IsUpper(r) && prevLower {
			b.WriteRune('-')
		}
		b.WriteRune(unicode.ToLower(r))
		prevLower = unicode.IsLower(r)
	}
	s := b.String()
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

func main() {
	schemesDir := flag.String("schemes", "cmd/genthemes/schemes", "directory of Windows Terminal color-scheme JSON files")
	out := flag.String("out", "internal/ui/themes_generated.go", "output Go source file")
	flag.Parse()

	entries, err := os.ReadDir(*schemesDir)
	if err != nil {
		fatal(err)
	}

	var got []genEntry
	seen := map[string]string{}
	skipped := 0

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(*schemesDir, e.Name()))
		if err != nil {
			fatal(err)
		}
		var s scheme
		if err := json.Unmarshal(raw, &s); err != nil {
			fmt.Fprintf(os.Stderr, "warn: parse %s: %v\n", e.Name(), err)
			skipped++
			continue
		}
		if s.Name == "" || s.Background == "" || s.Foreground == "" {
			fmt.Fprintf(os.Stderr, "warn: incomplete scheme in %s\n", e.Name())
			skipped++
			continue
		}
		key := normalize(s.Name)
		if prev, dup := seen[key]; dup {
			fmt.Fprintf(os.Stderr, "skip duplicate %q (from %q, already from %q)\n", key, e.Name(), prev)
			skipped++
			continue
		}
		pal := derive(s)
		ratio := contrast(parseHex(pal.fg), parseHex(pal.bg))
		if ratio < 4.5 {
			fmt.Fprintf(os.Stderr, "skip %q (%s): fg/bg contrast %.2f < 4.5\n", key, s.Name, ratio)
			skipped++
			continue
		}
		seen[key] = e.Name()
		got = append(got, genEntry{key, s.Name, pal})
	}
	sort.Slice(got, func(i, j int) bool { return got[i].key < got[j].key })

	src := emit(got)
	formatted, err := format.Source([]byte(src))
	if err != nil {
		fatal(fmt.Errorf("format generated source: %w\n--- source ---\n%s", err, src))
	}
	if err := os.WriteFile(*out, formatted, 0o644); err != nil {
		fatal(err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s: %d themes (%d skipped)\n", *out, len(got), skipped)
}

func emit(entries []genEntry) string {
	var b strings.Builder
	b.WriteString("// Code generated by cmd/genthemes from iTerm2-Color-Schemes; DO NOT EDIT.\n\n")
	b.WriteString("package ui\n\n")
	b.WriteString("import \"github.com/charmbracelet/lipgloss\"\n\n")
	b.WriteString("// generatedThemes maps normalized theme names to palettes derived from\n")
	b.WriteString("// iTerm2-Color-Schemes (Windows Terminal JSON format) by cmd/genthemes.\n")
	b.WriteString("// Each palette maps the scheme's 16 ANSI colors onto gsql's semantic slots\n")
	b.WriteString("// (blue->primary, magenta->accent, green->success, red->err, yellow->warn,\n")
	b.WriteString("// cyan->mark, bright-black->muted); tint slots (borders, cursor row, status\n")
	b.WriteString("// bar, search/visual) are synthesized by interpolating between bg and fg,\n")
	b.WriteString("// with a light/dark branch for the unfocused border. Schemes whose fg/bg\n")
	b.WriteString("// contrast fails WCAG AA (< 4.5) are skipped at generation time. Curated\n")
	b.WriteString("// (hand-tuned) themes in themes.go take precedence over a generated entry\n")
	b.WriteString("// with the same normalized name.\n")
	b.WriteString("//\n")
	b.WriteString("// Source: https://github.com/mbadolato/iTerm2-Color-Schemes (Windows\n")
	b.WriteString("// Terminal JSON format). Individual themes retain their original authors'\n")
	b.WriteString("// licenses; see the upstream repo for per-theme attribution.\n")
	b.WriteString("var generatedThemes = map[string]colorPalette{\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "\t%q: {\n", e.key)
		p := e.pal
		for _, f := range []struct{ name, val string }{
			{"primary", p.primary}, {"accent", p.accent}, {"success", p.success},
			{"mark", p.mark}, {"search", p.search}, {"visual", p.visual},
			{"cursorRow", p.cursorRow}, {"edit", p.edit}, {"warn", p.warn},
			{"err", p.err}, {"muted", p.muted}, {"label", p.label},
			{"border", p.border}, {"borderUnfocused", p.borderUnfocused},
			{"bg", p.bg}, {"stripe", p.stripe}, {"fg", p.fg},
			{"highlight", p.highlight}, {"statusBarBg", p.statusBarBg},
		} {
			fmt.Fprintf(&b, "\t\t%s: lipgloss.Color(%q),\n", f.name, f.val)
		}
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n")

	b.WriteString("\n// generatedDisplayNames maps each generated theme key to its original\n")
	b.WriteString("// (display) name from the upstream scheme, shown in the picker.\n")
	b.WriteString("var generatedDisplayNames = map[string]string{\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "\t%q: %q,\n", e.key, e.display)
	}
	b.WriteString("}\n")
	return b.String()
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "genthemes: %v\n", err)
	os.Exit(1)
}

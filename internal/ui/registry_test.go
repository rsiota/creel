package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestRegistryConsistency checks the registry itself is well-formed: every
// binding has a display string, a description, at least one dispatch token,
// and no duplicate entries within a section.
func TestRegistryConsistency(t *testing.T) {
	for _, sec := range registry() {
		if sec.Title == "" {
			t.Error("section with empty Title")
		}
		if sec.Source == "" {
			t.Errorf("section %q has no Source files", sec.Title)
		}
		seen := map[string]bool{}
		for _, b := range sec.Items {
			if b.Display == "" {
				t.Errorf("section %q has a binding with empty Display", sec.Title)
			}
			if b.Desc == "" {
				t.Errorf("section %q binding %q has empty Desc", sec.Title, b.Display)
			}
			if len(b.Tokens) == 0 {
				t.Errorf("section %q binding %q has no Tokens", sec.Title, b.Display)
			}
			if seen[b.Display] {
				t.Errorf("section %q has duplicate binding %q", sec.Title, b.Display)
			}
			seen[b.Display] = true
		}
	}
}

// TestKeybindingsMatchDispatch is the drift guard. For every documented
// binding token it asserts the token is actually handled by the key dispatch
// in the section's declared source files. This catches the most common form
// of drift: a keybinding documented in help that no longer (or never did)
// exist in the code.
//
// Tokens are collected from two mechanisms the codebase uses:
//   - string literals in `case "x":` clauses (switch msg.String())
//   - string arguments to key.WithKeys(...) (used for ctrl+q / ctrl+c)
func TestKeybindingsMatchDispatch(t *testing.T) {
	cache := map[string]map[string]bool{}
	avail := func(src string) map[string]bool {
		if m, ok := cache[src]; ok {
			return m
		}
		m := map[string]bool{}
		for _, file := range strings.Fields(src) {
			for tok := range collectDispatchTokens(t, file) {
				m[tok] = true
			}
		}
		cache[src] = m
		return m
	}

	for _, sec := range registry() {
		handled := avail(sec.Source)
		for _, b := range sec.Items {
			for _, tok := range b.Tokens {
				if !handled[tok] {
					t.Errorf("section %q, %q (%q): token %q not handled in dispatch (%s)",
						sec.Title, b.Display, b.Desc, tok, sec.Source)
				}
			}
		}
	}
}

// collectDispatchTokens parses a source file in this package and returns the
// set of key tokens the dispatch handles (case-clause literals plus
// key.WithKeys arguments).
func collectDispatchTokens(t *testing.T, name string) map[string]bool {
	t.Helper()
	path := sourceFile(t, name)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	tokens := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		switch nn := n.(type) {
		case *ast.CaseClause:
			for _, v := range nn.List {
				addStringLit(tokens, v)
			}
		case *ast.CallExpr:
			// key.WithKeys("ctrl+c") — collect its string-literal args.
			if sel, ok := nn.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "WithKeys" {
				for _, arg := range nn.Args {
					addStringLit(tokens, arg)
				}
			}
		case *ast.BinaryExpr:
			// Chord handlers dispatch as msg.String() == "x" (e.g. the g r / g R
			// chords), which a case clause can't express. Collect the string
			// operand of such a comparison so documented chords are recognised.
			if nn.Op == token.EQL {
				if isStringMethodCall(nn.X) {
					addStringLit(tokens, nn.Y)
				} else if isStringMethodCall(nn.Y) {
					addStringLit(tokens, nn.X)
				}
			}
		}
		return true
	})
	return tokens
}

// isStringMethodCall reports whether e is a foo.String() call — the left side
// of the chord-handler comparison msg.String() == "x".
func isStringMethodCall(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "String"
}

// addStringLit records a string-literal expression into the token set.
func addStringLit(set map[string]bool, e ast.Expr) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return
	}
	v, err := strconv.Unquote(lit.Value)
	if err != nil {
		return
	}
	set[v] = true
}

// sourceFile resolves a package-relative basename to an absolute path next to
// this test file.
func sourceFile(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(thisFile), name)
}

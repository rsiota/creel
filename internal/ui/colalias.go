package ui

import (
	"fmt"
	"strings"

	"github.com/rsiota/creel/internal/db"
)

// Duplicate result names (SELECT * across a JOIN, or two unaliased id columns)
// break the grid, which keys hidden columns, sorts, and filters by name, and
// they break MySQL, which rejects a derived table whose columns are not unique.
//
// rewriteSelectAliases rewrites the select list so each output name is unique:
// the first id stays id, the next becomes orders.id (or id_2 when the source
// isn't known). Star lists are expanded from table schemas only when a name
// actually collides. When the rewrite isn't possible, disambiguateColumnNames
// still gives the headers distinct labels and colsDisambiguated keeps
// filter/sort off, because the SQL names are unchanged.

// disambiguateColumnNames returns display names that are unique ignoring case.
// The first occurrence of a name is kept; later copies become name_2, name_3,
// skipping suffixes that are already taken. changed is false when names was
// already unique.
func disambiguateColumnNames(names []string) (out []string, changed bool) {
	out = make([]string, len(names))
	bases := make([]string, len(names))
	for i, name := range names {
		base := strings.TrimSpace(name)
		if base == "" {
			base = "column"
		}
		bases[i] = base
	}
	used := map[string]bool{}
	seen := map[string]bool{}
	for _, base := range bases {
		key := strings.ToLower(base)
		if !seen[key] {
			seen[key] = true
			used[key] = true
		}
	}
	emitted := map[string]int{}
	for i, base := range bases {
		key := strings.ToLower(base)
		if emitted[key] == 0 {
			out[i] = base
		} else {
			out[i] = pickDupName(base, "", used)
			used[strings.ToLower(out[i])] = true
		}
		emitted[key]++
		if out[i] != names[i] {
			changed = true
		}
	}
	return out, changed
}

// columnLeaf is the name after the last dot, so a qualified duplicate
// (orders.status, u.created_at) still matches status and datetime heuristics.
func columnLeaf(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.LastIndex(name, "."); i >= 0 {
		if leaf := strings.TrimSpace(name[i+1:]); leaf != "" {
			return leaf
		}
	}
	return name
}

// rewriteSelectAliases returns query with duplicate output names aliased.
// ok is false when the statement needs no change or cannot be rewritten safely.
// colsOf supplies column names in SELECT * order for a table (schema.table or
// table). It is called only when a star list might collide.
func rewriteSelectAliases(query string, driver db.Driver, colsOf func(string) ([]string, bool)) (string, bool) {
	q := strings.TrimSpace(query)
	q = strings.TrimRight(q, ";")
	if q == "" || !strings.HasPrefix(strings.ToUpper(q), "SELECT") {
		return "", false
	}
	headEnd, fromAt, ok := selectListSpan(q)
	if !ok {
		return "", false
	}
	parts, ok := splitSelectList(q[headEnd:fromAt])
	if !ok || len(parts) == 0 {
		return "", false
	}
	items := make([]selItem, len(parts))
	hasStar := false
	for i, part := range parts {
		item, ok := classifySelectItem(part)
		if !ok {
			return "", false
		}
		items[i] = item
		if item.kind != kindExpr {
			hasStar = true
		}
	}
	if !hasStar && !duplicateOutputs(items) {
		return "", false
	}

	var sources []fromSource
	if hasStar {
		sources, ok = parseFromSources(q[fromAt:])
		if !ok || len(sources) == 0 {
			return "", false
		}
		if singleTableStar(items, sources) {
			return "", false
		}
	}

	slots, ok := buildSlots(items, sources, colsOf, driver)
	if !ok {
		return "", false
	}
	names := assignSlotNames(slots)
	if !slotNamesChanged(slots, names) {
		return "", false
	}
	rewritten := q[:headEnd] + emitSelectList(items, slots, names, driver) + " " + q[fromAt:]
	return rewritten, true
}

// clearAliasState drops a previous statement's rewrite so the next query
// cannot filter through stale SQL.
func (m *Model) clearAliasState() {
	m.wrapSource = ""
	m.colsDisambiguated = false
}

// aliasedSource is the statement to re-run for export and full-result charts.
// When query is still the user's original text, the rewritten form (unique
// output names) is used. A filtered or sorted lastQuery is already wrapped.
func (m Model) aliasedSource(query string) string {
	q := strings.TrimRight(strings.TrimSpace(query), ";")
	wrap := strings.TrimRight(strings.TrimSpace(m.wrapSource), ";")
	if wrap == "" {
		return q
	}
	base := strings.TrimRight(strings.TrimSpace(m.baseQuery), ";")
	if q == base {
		return wrap
	}
	return q
}

// resultColumnLookup resolves table columns from the sidebar cache, then the
// live connection. The cache map is snapshotted so the returned func is safe
// to call from the query goroutine.
func (m *Model) resultColumnLookup() func(string) ([]string, bool) {
	cache := snapshotColumnNames(m.columnCache)
	var database db.DB
	if m.connection != nil {
		database = m.connection.DB()
	}
	return func(table string) ([]string, bool) {
		if cols, ok := lookupCachedColumns(cache, table); ok {
			return cols, true
		}
		if database == nil {
			return nil, false
		}
		fetched, err := database.TableSchema(table)
		if (err != nil || len(fetched) == 0) && strings.Contains(table, ".") {
			if i := strings.LastIndex(table, "."); i >= 0 {
				fetched, err = database.TableSchema(table[i+1:])
			}
		}
		if err != nil || len(fetched) == 0 {
			return nil, false
		}
		names := make([]string, len(fetched))
		for i, col := range fetched {
			names[i] = col.Name
		}
		return names, true
	}
}

func snapshotColumnNames(cache map[string][]db.Column) map[string][]string {
	if len(cache) == 0 {
		return nil
	}
	out := make(map[string][]string, len(cache))
	for table, cols := range cache {
		names := make([]string, len(cols))
		for i, col := range cols {
			names[i] = col.Name
		}
		out[table] = names
	}
	return out
}

func lookupCachedColumns(cache map[string][]string, table string) ([]string, bool) {
	if len(cache) == 0 || table == "" {
		return nil, false
	}
	if cols, ok := cache[table]; ok && len(cols) > 0 {
		return cols, true
	}
	for key, cols := range cache {
		if strings.EqualFold(key, table) && len(cols) > 0 {
			return cols, true
		}
	}
	if i := strings.LastIndex(table, "."); i >= 0 {
		return lookupCachedColumns(cache, table[i+1:])
	}
	return nil, false
}

type itemKind int

const (
	kindExpr itemKind = iota
	kindStar
	kindQualStar
)

type selItem struct {
	raw     string
	kind    itemKind
	qual    string // qualifier of qual.*
	expr    string // expression without an output alias
	outName string
	prefix  string // table/alias of a plain column ref, for dup labels
}

type fromSource struct {
	lookup   string
	ref      string
	refParts []string
}

type outSlot struct {
	item    int
	natural string
	qual    string
	expr    string
}

func duplicateOutputs(items []selItem) bool {
	seen := map[string]bool{}
	for _, item := range items {
		if item.kind != kindExpr {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(item.outName))
		if key == "" || seen[key] {
			return true
		}
		seen[key] = true
	}
	return false
}

func singleTableStar(items []selItem, sources []fromSource) bool {
	if len(sources) != 1 || len(items) != 1 || items[0].kind != kindStar {
		return false
	}
	return true
}

func buildSlots(items []selItem, sources []fromSource, colsOf func(string) ([]string, bool), driver db.Driver) ([]outSlot, bool) {
	var slots []outSlot
	for i, item := range items {
		switch item.kind {
		case kindExpr:
			slots = append(slots, outSlot{
				item:    i,
				natural: item.outName,
				qual:    item.prefix,
				expr:    item.expr,
			})
		case kindStar:
			for _, src := range sources {
				cols, ok := colsOf(src.lookup)
				if !ok || len(cols) == 0 {
					return nil, false
				}
				for _, col := range cols {
					slots = append(slots, outSlot{
						item:    i,
						natural: col,
						qual:    src.ref,
						expr:    quoteColRef(driver, src.refParts, col),
					})
				}
			}
		case kindQualStar:
			src, ok := matchSource(sources, item.qual)
			if !ok {
				return nil, false
			}
			cols, ok := colsOf(src.lookup)
			if !ok || len(cols) == 0 {
				return nil, false
			}
			for _, col := range cols {
				slots = append(slots, outSlot{
					item:    i,
					natural: col,
					qual:    src.ref,
					expr:    quoteColRef(driver, src.refParts, col),
				})
			}
		}
	}
	if len(slots) == 0 {
		return nil, false
	}
	return slots, true
}

func matchSource(sources []fromSource, qual string) (fromSource, bool) {
	for _, src := range sources {
		if strings.EqualFold(src.ref, qual) || strings.EqualFold(src.lookup, qual) {
			return src, true
		}
	}
	return fromSource{}, false
}

func assignSlotNames(slots []outSlot) []string {
	used := map[string]bool{}
	seen := map[string]bool{}
	for _, slot := range slots {
		base := slotBase(slot.natural)
		key := strings.ToLower(base)
		if !seen[key] {
			seen[key] = true
			used[key] = true
		}
	}
	names := make([]string, len(slots))
	emitted := map[string]int{}
	for i, slot := range slots {
		base := slotBase(slot.natural)
		key := strings.ToLower(base)
		if emitted[key] == 0 {
			names[i] = base
			emitted[key]++
			continue
		}
		emitted[key]++
		names[i] = pickDupName(base, slot.qual, used)
		used[strings.ToLower(names[i])] = true
	}
	return names
}

func slotBase(natural string) string {
	base := strings.TrimSpace(natural)
	if base == "" {
		return "column"
	}
	return base
}

func pickDupName(base, qual string, used map[string]bool) string {
	if qual != "" {
		cand := qual + "." + base
		if !used[strings.ToLower(cand)] {
			return cand
		}
	}
	for n := 2; ; n++ {
		cand := fmt.Sprintf("%s_%d", base, n)
		if !used[strings.ToLower(cand)] {
			return cand
		}
	}
}

func slotNamesChanged(slots []outSlot, names []string) bool {
	for i, slot := range slots {
		if names[i] != slot.natural {
			return true
		}
	}
	return false
}

func emitSelectList(items []selItem, slots []outSlot, names []string, driver db.Driver) string {
	var b strings.Builder
	first := true
	write := func(sql string) {
		if !first {
			b.WriteString(", ")
		}
		first = false
		b.WriteString(sql)
	}
	at := 0
	for i, item := range items {
		var mine []int
		for at < len(slots) && slots[at].item == i {
			mine = append(mine, at)
			at++
		}
		if len(mine) == 0 {
			continue
		}
		expand := false
		for _, si := range mine {
			if names[si] != slots[si].natural {
				expand = true
				break
			}
		}
		if item.kind == kindExpr {
			if !expand {
				write(strings.TrimSpace(item.raw))
			} else {
				write(item.expr + " AS " + quoteIdentD(driver, names[mine[0]]))
			}
			continue
		}
		if !expand {
			write(strings.TrimSpace(item.raw))
			continue
		}
		for _, si := range mine {
			piece := slots[si].expr
			if names[si] != slots[si].natural {
				piece += " AS " + quoteIdentD(driver, names[si])
			}
			write(piece)
		}
	}
	return b.String()
}

func quoteColRef(driver db.Driver, parts []string, col string) string {
	all := make([]string, 0, len(parts)+1)
	all = append(all, parts...)
	all = append(all, col)
	quoted := make([]string, len(all))
	for i, part := range all {
		quoted[i] = quoteIdentD(driver, part)
	}
	return strings.Join(quoted, ".")
}

func classifySelectItem(raw string) (selItem, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return selItem{}, false
	}
	if s == "*" {
		return selItem{raw: raw, kind: kindStar}, true
	}
	if qual, ok := qualifiedStar(s); ok {
		return selItem{raw: raw, kind: kindQualStar, qual: qual}, true
	}
	if expr, alias, ok := splitOutputAlias(s); ok {
		prefix, _, _ := plainColumnRef(expr)
		return selItem{raw: raw, kind: kindExpr, expr: expr, outName: alias, prefix: prefix}, true
	}
	if prefix, name, ok := plainColumnRef(s); ok {
		return selItem{raw: raw, kind: kindExpr, expr: s, outName: name, prefix: prefix}, true
	}
	return selItem{}, false
}

func qualifiedStar(s string) (string, bool) {
	toks, ok := tokenizeItem(s)
	if !ok || len(toks) < 3 || toks[len(toks)-1].kind != tokStar {
		return "", false
	}
	if toks[len(toks)-2].kind != tokDot {
		return "", false
	}
	var parts []string
	for i := 0; i < len(toks)-2; i++ {
		if i%2 == 0 {
			if toks[i].kind != tokIdent {
				return "", false
			}
			parts = append(parts, toks[i].text)
			continue
		}
		if toks[i].kind != tokDot {
			return "", false
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, "."), true
}

func plainColumnRef(s string) (prefix, name string, ok bool) {
	toks, good := tokenizeItem(s)
	if !good || len(toks) == 0 {
		return "", "", false
	}
	var parts []string
	for i, tok := range toks {
		if i%2 == 0 {
			if tok.kind != tokIdent {
				return "", "", false
			}
			parts = append(parts, tok.text)
			continue
		}
		if tok.kind != tokDot {
			return "", "", false
		}
	}
	if len(parts) == 0 {
		return "", "", false
	}
	name = parts[len(parts)-1]
	if len(parts) > 1 {
		prefix = strings.Join(parts[:len(parts)-1], ".")
	}
	return prefix, name, true
}

func splitOutputAlias(s string) (expr, alias string, ok bool) {
	toks, good := tokenizeItem(s)
	if !good || len(toks) == 0 {
		return "", "", false
	}
	depth := 0
	asAt := -1
	for i, tok := range toks {
		switch tok.kind {
		case tokLParen:
			depth++
		case tokRParen:
			depth--
			if depth < 0 {
				return "", "", false
			}
		case tokIdent:
			if depth == 0 && strings.EqualFold(tok.text, "AS") {
				if asAt >= 0 {
					return "", "", false
				}
				asAt = i
			}
		}
	}
	if asAt >= 0 {
		if asAt == 0 || asAt != len(toks)-2 || toks[asAt+1].kind != tokIdent {
			return "", "", false
		}
		expr = strings.TrimSpace(s[:toks[asAt].pos])
		if expr == "" {
			return "", "", false
		}
		return expr, toks[asAt+1].text, true
	}
	if len(toks) < 2 || toks[len(toks)-1].kind != tokIdent {
		return "", "", false
	}
	prev := toks[len(toks)-2]
	if prev.kind == tokDot {
		return "", "", false
	}
	between := s[prev.end:toks[len(toks)-1].pos]
	if !strings.ContainsAny(between, " \t\n\r") {
		return "", "", false
	}
	expr = strings.TrimSpace(s[:toks[len(toks)-1].pos])
	if expr == "" {
		return "", "", false
	}
	return expr, toks[len(toks)-1].text, true
}

const (
	tokIdent = iota
	tokDot
	tokStar
	tokLParen
	tokRParen
	tokOther
)

type itemTok struct {
	kind int
	text string
	pos  int
	end  int
}

func tokenizeItem(s string) ([]itemTok, bool) {
	var toks []itemTok
	i := 0
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
			i++
		}
		if i >= len(s) {
			break
		}
		start := i
		c := s[i]
		switch c {
		case '\'', '"', '`', '[':
			name, next, ok := scanQuoted(s, i)
			if !ok {
				return nil, false
			}
			toks = append(toks, itemTok{kind: tokIdent, text: name, pos: start, end: next})
			i = next
		case '.':
			toks = append(toks, itemTok{kind: tokDot, text: ".", pos: start, end: i + 1})
			i++
		case '*':
			toks = append(toks, itemTok{kind: tokStar, text: "*", pos: start, end: i + 1})
			i++
		case '(':
			toks = append(toks, itemTok{kind: tokLParen, text: "(", pos: start, end: i + 1})
			i++
		case ')':
			toks = append(toks, itemTok{kind: tokRParen, text: ")", pos: start, end: i + 1})
			i++
		default:
			if isIdentStart(c) {
				j := i + 1
				for j < len(s) && isIdentCont(s[j]) {
					j++
				}
				toks = append(toks, itemTok{kind: tokIdent, text: s[i:j], pos: start, end: j})
				i = j
				continue
			}
			if c >= 128 {
				return nil, false
			}
			toks = append(toks, itemTok{kind: tokOther, text: s[i : i+1], pos: start, end: i + 1})
			i++
		}
	}
	return toks, true
}

func selectListSpan(query string) (headEnd, fromAt int, ok bool) {
	sc := &sqlScanner{s: query}
	sc.skipSpace()
	if !sc.consumeKeyword("SELECT") || sc.bad {
		return 0, 0, false
	}
	sc.skipSpace()
	if sc.consumeKeyword("DISTINCT") {
		sc.skipSpace()
		if sc.consumeKeyword("ON") || sc.bad {
			return 0, 0, false
		}
	} else {
		sc.consumeKeyword("ALL")
		sc.skipSpace()
	}
	if sc.bad {
		return 0, 0, false
	}
	headEnd = sc.i
	depth := 0
	for sc.i < len(sc.s) {
		c := sc.s[sc.i]
		if c == '-' && sc.i+1 < len(sc.s) && sc.s[sc.i+1] == '-' {
			return 0, 0, false
		}
		if c == '/' && sc.i+1 < len(sc.s) && sc.s[sc.i+1] == '*' {
			return 0, 0, false
		}
		if c == '$' {
			return 0, 0, false
		}
		if c == '\'' || c == '"' || c == '`' || c == '[' {
			if !sc.skipQuoted() {
				return 0, 0, false
			}
			continue
		}
		if c == '(' {
			depth++
			sc.i++
			continue
		}
		if c == ')' {
			if depth == 0 {
				return 0, 0, false
			}
			depth--
			sc.i++
			continue
		}
		if depth == 0 && sc.atKeyword("FROM") {
			return headEnd, sc.i, true
		}
		sc.i++
	}
	return 0, 0, false
}

func splitSelectList(list string) ([]string, bool) {
	var items []string
	start := 0
	sc := &sqlScanner{s: list}
	depth := 0
	for sc.i < len(sc.s) {
		c := sc.s[sc.i]
		if c == '-' && sc.i+1 < len(sc.s) && sc.s[sc.i+1] == '-' {
			return nil, false
		}
		if c == '/' && sc.i+1 < len(sc.s) && sc.s[sc.i+1] == '*' {
			return nil, false
		}
		if c == '$' {
			return nil, false
		}
		if c == '\'' || c == '"' || c == '`' || c == '[' {
			if !sc.skipQuoted() {
				return nil, false
			}
			continue
		}
		if c == '(' {
			depth++
			sc.i++
			continue
		}
		if c == ')' {
			if depth == 0 {
				return nil, false
			}
			depth--
			sc.i++
			continue
		}
		if c == ',' && depth == 0 {
			item := strings.TrimSpace(list[start:sc.i])
			if item == "" {
				return nil, false
			}
			items = append(items, item)
			sc.i++
			start = sc.i
			continue
		}
		sc.i++
	}
	if depth != 0 {
		return nil, false
	}
	last := strings.TrimSpace(list[start:])
	if last == "" {
		return nil, false
	}
	items = append(items, last)
	return items, true
}

func parseFromSources(fromClause string) ([]fromSource, bool) {
	sc := &sqlScanner{s: fromClause}
	sc.skipSpace()
	if !sc.consumeKeyword("FROM") || sc.bad {
		return nil, false
	}
	var sources []fromSource
	for {
		sc.skipSpace()
		if sc.bad || sc.eof() {
			break
		}
		if sc.peekByte() == '(' || sc.atKeyword("LATERAL") {
			return nil, false
		}
		if sc.atKeyword("NATURAL") {
			return nil, false
		}
		parts, ok := sc.readDottedIdent()
		if !ok || sc.bad {
			return nil, false
		}
		src := fromSource{
			lookup:   strings.Join(parts, "."),
			ref:      strings.Join(parts, "."),
			refParts: parts,
		}
		sc.skipSpace()
		if sc.consumeKeyword("AS") {
			alias, ok := sc.readIdent()
			if !ok {
				return nil, false
			}
			src.ref = alias
			src.refParts = []string{alias}
		} else if id, ok := sc.peekIdent(); ok && !fromStopWord(id) {
			sc.readIdent()
			src.ref = id
			src.refParts = []string{id}
		}
		if sc.bad {
			return nil, false
		}
		sources = append(sources, src)
		sc.skipSpace()
		if sc.atKeyword("USING") || sc.atKeyword("NATURAL") {
			return nil, false
		}
		if sc.consumeKeyword("ON") {
			if !sc.skipOnCondition() {
				return nil, false
			}
			sc.skipSpace()
		}
		if sc.eof() || sc.peekByte() == ';' {
			break
		}
		if sc.peekByte() == ',' {
			sc.i++
			continue
		}
		if sc.atJoinHead() {
			if !sc.consumeJoinHead() {
				return nil, false
			}
			continue
		}
		if id, ok := sc.peekIdent(); ok && fromClauseEnd(id) {
			break
		}
		if sc.eof() {
			break
		}
		if _, ok := sc.peekIdent(); ok {
			return nil, false
		}
		break
	}
	if sc.bad || len(sources) == 0 {
		return nil, false
	}
	return sources, true
}

func fromStopWord(id string) bool {
	return fromClauseEnd(id) || isJoinWord(id) || strings.EqualFold(id, "ON") ||
		strings.EqualFold(id, "USING") || strings.EqualFold(id, "AS") ||
		strings.EqualFold(id, "NATURAL") || strings.EqualFold(id, "SET")
}

func fromClauseEnd(id string) bool {
	switch strings.ToUpper(id) {
	case "WHERE", "GROUP", "ORDER", "LIMIT", "OFFSET", "HAVING", "UNION",
		"EXCEPT", "INTERSECT", "WINDOW", "FETCH", "FOR", "RETURNING":
		return true
	default:
		return false
	}
}

func isJoinWord(id string) bool {
	switch strings.ToUpper(id) {
	case "JOIN", "INNER", "LEFT", "RIGHT", "FULL", "CROSS", "OUTER":
		return true
	default:
		return false
	}
}

type sqlScanner struct {
	s   string
	i   int
	bad bool
}

func (sc *sqlScanner) eof() bool {
	return sc.i >= len(sc.s)
}

func (sc *sqlScanner) peekByte() byte {
	if sc.eof() {
		return 0
	}
	return sc.s[sc.i]
}

func (sc *sqlScanner) skipSpace() {
	for sc.i < len(sc.s) && !sc.bad {
		c := sc.s[sc.i]
		switch c {
		case ' ', '\t', '\n', '\r':
			sc.i++
		case '-':
			if sc.i+1 < len(sc.s) && sc.s[sc.i+1] == '-' {
				sc.i += 2
				for sc.i < len(sc.s) && sc.s[sc.i] != '\n' {
					sc.i++
				}
				continue
			}
			return
		case '/':
			if sc.i+1 < len(sc.s) && sc.s[sc.i+1] == '*' {
				sc.i += 2
				for sc.i+1 < len(sc.s) && !(sc.s[sc.i] == '*' && sc.s[sc.i+1] == '/') {
					sc.i++
				}
				if sc.i+1 >= len(sc.s) {
					sc.bad = true
					sc.i = len(sc.s)
					return
				}
				sc.i += 2
				continue
			}
			return
		default:
			return
		}
	}
}

func (sc *sqlScanner) atKeyword(kw string) bool {
	n := len(kw)
	if sc.i+n > len(sc.s) {
		return false
	}
	if !strings.EqualFold(sc.s[sc.i:sc.i+n], kw) {
		return false
	}
	if sc.i > 0 && isIdentCont(sc.s[sc.i-1]) {
		return false
	}
	end := sc.i + n
	if end < len(sc.s) && isIdentCont(sc.s[end]) {
		return false
	}
	return true
}

func (sc *sqlScanner) consumeKeyword(kw string) bool {
	sc.skipSpace()
	if !sc.atKeyword(kw) {
		return false
	}
	sc.i += len(kw)
	return true
}

func (sc *sqlScanner) readIdent() (string, bool) {
	sc.skipSpace()
	if sc.bad || sc.eof() {
		return "", false
	}
	c := sc.s[sc.i]
	if c == '"' || c == '`' || c == '[' || c == '\'' {
		if c == '\'' {
			return "", false
		}
		name, next, ok := scanQuoted(sc.s, sc.i)
		if !ok {
			sc.bad = true
			return "", false
		}
		sc.i = next
		return name, true
	}
	if !isIdentStart(c) {
		return "", false
	}
	start := sc.i
	sc.i++
	for sc.i < len(sc.s) && isIdentCont(sc.s[sc.i]) {
		sc.i++
	}
	return sc.s[start:sc.i], true
}

func (sc *sqlScanner) readDottedIdent() ([]string, bool) {
	first, ok := sc.readIdent()
	if !ok {
		return nil, false
	}
	parts := []string{first}
	for {
		save := sc.i
		sc.skipSpace()
		if sc.eof() || sc.s[sc.i] != '.' {
			sc.i = save
			break
		}
		sc.i++
		next, ok := sc.readIdent()
		if !ok {
			return nil, false
		}
		parts = append(parts, next)
	}
	return parts, true
}

func (sc *sqlScanner) peekIdent() (string, bool) {
	save, bad := sc.i, sc.bad
	id, ok := sc.readIdent()
	sc.i, sc.bad = save, bad
	return id, ok
}

func (sc *sqlScanner) atJoinHead() bool {
	id, ok := sc.peekIdent()
	if !ok {
		return false
	}
	switch strings.ToUpper(id) {
	case "JOIN", "INNER", "LEFT", "RIGHT", "FULL", "CROSS":
		return true
	default:
		return false
	}
}

func (sc *sqlScanner) consumeJoinHead() bool {
	sc.skipSpace()
	id, ok := sc.peekIdent()
	if !ok {
		return false
	}
	switch strings.ToUpper(id) {
	case "JOIN":
		return sc.consumeKeyword("JOIN")
	case "INNER", "CROSS":
		sc.readIdent()
		return sc.consumeKeyword("JOIN")
	case "LEFT", "RIGHT", "FULL":
		sc.readIdent()
		sc.skipSpace()
		sc.consumeKeyword("OUTER")
		return sc.consumeKeyword("JOIN")
	default:
		return false
	}
}

func (sc *sqlScanner) skipOnCondition() bool {
	for !sc.eof() && !sc.bad {
		sc.skipSpace()
		if sc.eof() {
			return true
		}
		c := sc.s[sc.i]
		if c == '\'' || c == '"' || c == '`' || c == '[' {
			if !sc.skipQuoted() {
				return false
			}
			continue
		}
		if c == '$' {
			return false
		}
		if c == '(' {
			if !sc.skipParens() {
				return false
			}
			continue
		}
		if id, ok := sc.peekIdent(); ok {
			up := strings.ToUpper(id)
			switch up {
			case "JOIN", "INNER", "LEFT", "RIGHT", "FULL", "CROSS", "NATURAL",
				"WHERE", "GROUP", "ORDER", "LIMIT", "OFFSET", "HAVING", "UNION",
				"EXCEPT", "INTERSECT", "WINDOW", "FETCH":
				return true
			}
			sc.readIdent()
			continue
		}
		sc.i++
	}
	return !sc.bad
}

func (sc *sqlScanner) skipParens() bool {
	if sc.eof() || sc.s[sc.i] != '(' {
		return false
	}
	depth := 0
	for sc.i < len(sc.s) {
		c := sc.s[sc.i]
		if c == '\'' || c == '"' || c == '`' || c == '[' {
			if !sc.skipQuoted() {
				return false
			}
			continue
		}
		if c == '(' {
			depth++
			sc.i++
			continue
		}
		if c == ')' {
			depth--
			sc.i++
			if depth == 0 {
				return true
			}
			continue
		}
		sc.i++
	}
	return false
}

func (sc *sqlScanner) skipQuoted() bool {
	if sc.eof() {
		return false
	}
	_, next, ok := scanQuoted(sc.s, sc.i)
	if !ok {
		sc.bad = true
		return false
	}
	sc.i = next
	return true
}

func scanQuoted(s string, i int) (name string, next int, ok bool) {
	if i >= len(s) {
		return "", i, false
	}
	open := s[i]
	close := open
	if open == '[' {
		close = ']'
	}
	i++
	start := i
	var b strings.Builder
	for i < len(s) {
		if s[i] == close {
			if open != '[' && i+1 < len(s) && s[i+1] == close {
				b.WriteString(s[start:i])
				b.WriteByte(close)
				i += 2
				start = i
				continue
			}
			b.WriteString(s[start:i])
			return b.String(), i + 1, true
		}
		i++
	}
	return "", len(s), false
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isIdentCont(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

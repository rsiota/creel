#!/usr/bin/env bash
# Feature smoke: unit tests + CLI checks against the demo DB.
# TUI-only surfaces (explorer, ERD, charts, …) are listed at the end for a
# short manual pass — or re-run `vhs demo/graph-tour.tape` for the graph loop.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CREEL="${CREEL:-$ROOT/creel}"
DB="${CREEL_SMOKE_DB:-$ROOT/demo/creel-demo.db}"
fail=0

pass() { printf '  ok  %s\n' "$*"; }
bad()  { printf '  FAIL %s\n' "$*"; fail=$((fail + 1)); }

need() {
  if ! "$@"; then
    bad "$*"
    return 1
  fi
  pass "$*"
}

echo "== build =="
go build -o "$CREEL" ./cmd/creel
test -x "$CREEL"
pass "go build ./cmd/creel"

if [[ ! -f "$DB" ]]; then
  echo "building demo DB…"
  sqlite3 "$DB" < demo/schema.sql
fi
pass "demo DB at $DB"

echo
echo "== unit tests =="
if go test ./... -count=1; then
  pass "go test ./..."
else
  bad "go test ./..."
fi

echo
echo "== CLI smoke (demo DB) =="

out=$("$CREEL" -version 2>&1) || true
need grep -q creel <<<"$out"

# Basic SELECT + formats
for fmt in tsv csv json jsonl md; do
  if "$CREEL" -database "$DB" -e "SELECT id, name, role FROM users ORDER BY id LIMIT 2" -format "$fmt" >/tmp/creel-smoke."$fmt" 2>/tmp/creel-smoke.err; then
    pass "SELECT users -format $fmt"
  else
    bad "SELECT users -format $fmt ($(cat /tmp/creel-smoke.err))"
  fi
done
need grep -q 'Ada Lovelace' /tmp/creel-smoke.tsv
need grep -q 'Ada Lovelace' /tmp/creel-smoke.csv
need grep -q '"name"' /tmp/creel-smoke.json

# URI form
if "$CREEL" -uri "sqlite:$DB" -e "SELECT count(*) AS n FROM users" -format tsv 2>/dev/null | grep -q 8; then
  pass "sqlite URI + count(users)=8"
else
  bad "sqlite URI + count"
fi

# stdin query
if echo "SELECT name FROM products ORDER BY id LIMIT 1" | "$CREEL" -database "$DB" -e - -format tsv 2>/dev/null | grep -qi .; then
  pass "-e - stdin query"
else
  bad "-e - stdin query"
fi

# read-only rejects writes
if ! "$CREEL" -database "$DB" -readonly -e "INSERT INTO users (email, name, role) VALUES ('x@x','x','customer')" 2>/tmp/creel-ro.err; then
  pass "-readonly rejects INSERT"
else
  bad "-readonly should reject INSERT"
fi

# failure exit code
set +e
"$CREEL" -database "$DB" -e "SELECT * FROM no_such_table" >/dev/null 2>&1
ec=$?
set -e
if [[ $ec -eq 1 ]]; then
  pass "SQL error exits 1"
else
  bad "SQL error exit=$ec want 1"
fi

# FK-rich queries used by graph/chart features
if "$CREEL" -database "$DB" -e "SELECT o.id, u.name, o.status, o.total FROM orders o JOIN users u ON u.id = o.user_id ORDER BY o.id" -format tsv 2>/dev/null | grep -q .; then
  pass "JOIN orders↔users (chart/diff fodder)"
else
  bad "JOIN orders↔users"
fi

if "$CREEL" -database "$DB" -e "SELECT status, count(*) AS n FROM orders GROUP BY status" -format tsv 2>/dev/null | grep -q .; then
  pass "GROUP BY status (freq/pie fodder)"
else
  bad "GROUP BY status"
fi

# Schema introspection used by ERD / structure
tables=$("$CREEL" -database "$DB" -e "SELECT name FROM sqlite_master WHERE type='table' ORDER BY 1" -format tsv 2>/dev/null)
for t in users orders addresses products categories reviews payments order_items; do
  if grep -qx "$t" <<<"$tables"; then
    pass "table $t present"
  else
    bad "missing table $t"
  fi
done

# FK edges exist (relationship explorer / ERD)
fk=$("$CREEL" -database "$DB" -e "PRAGMA foreign_key_list(orders)" -format tsv 2>/dev/null || true)
if grep -q users <<<"$fk"; then
  pass "orders.user_id → users"
else
  bad "orders FK to users"
fi

echo
echo "== live MySQL/Postgres (optional) =="
if [[ "${CREEL_LIVE_DB:-}" == "1" ]]; then
  if go test ./internal/db/ -count=1 -run 'TestLive'; then
    pass "live driver tests"
  else
    bad "live driver tests"
  fi
else
  echo "  skip (set CREEL_LIVE_DB=1 with local servers, or rely on CI)"
fi

echo
echo "== TUI manual checklist (demo DB) =="
cat <<'EOF'
  Open:  creel -database demo/creel-demo.db
  Or:    creel   # empty picker → Try the demo database

  [ ] ? Start tab shows Graph & charts (g r / g R / M+:bar)
  [ ] Ctrl+P → "static ERD" / g R replays
  [ ] :goto users → results; g r explorer; A insert-related; esc; :explore
  [ ] :erd → p path → i JOIN in editor
  [ ] :goto orders → M on status → :pie / :freq / :bar
  [ ] g e EXPLAIN; g E EXPLAIN ANALYZE (confirm); :diagnose
  [ ] S / :grep cross-table search
  [ ] :diff after opening users in two tabs
  [ ] :watch 2 with a chart open (redraws)
  [ ] E cell edit; ctrl+s stage; :w save (then undo / discard)
  [ ] d structure; N table designer (cancel)
  [ ] g c theme picker; :zen; alt+b / alt+e
  [ ] Ctrl+F assistant (needs AI config) — or skip
  [ ] q quit

  Recorded graph loop:  vhs demo/graph-tour.tape
EOF

echo
if [[ $fail -eq 0 ]]; then
  echo "All automated smoke checks passed."
  exit 0
fi
echo "$fail automated check(s) failed."
exit 1

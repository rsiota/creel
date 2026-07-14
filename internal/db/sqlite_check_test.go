package db

import (
	"reflect"
	"testing"
)

func c(name, col, expr string) CheckConstraint {
	return CheckConstraint{Name: name, Column: col, Expression: expr}
}

func TestParseSQLiteCheckConstraints(t *testing.T) {
	tests := []struct {
		name string
		ddl  string
		want []CheckConstraint
	}{
		{
			name: "column-level check",
			ddl:  `CREATE TABLE users (id INTEGER PRIMARY KEY, age INTEGER CHECK (age >= 0))`,
			want: []CheckConstraint{c("", "age", "age >= 0")},
		},
		{
			name: "table-level check",
			ddl:  `CREATE TABLE t (id INTEGER, CHECK (status IN ('a','b')))`,
			want: []CheckConstraint{c("", "", "status IN ('a','b')")},
		},
		{
			name: "named table-level check",
			ddl:  `CREATE TABLE t (id INTEGER, CONSTRAINT chk_pos CHECK (x > 0))`,
			want: []CheckConstraint{c("chk_pos", "", "x > 0")},
		},
		{
			name: "mixed column and table checks",
			ddl:  `CREATE TABLE t (a INTEGER CHECK (a > 0), b TEXT, CONSTRAINT chk_b CHECK (b IS NOT NULL))`,
			want: []CheckConstraint{
				c("", "a", "a > 0"),
				c("chk_b", "", "b IS NOT NULL"),
			},
		},
		{
			name: "nested parens and commas inside expression",
			ddl:  `CREATE TABLE t (price REAL CHECK (price > 0 AND (currency = 'USD' OR currency = 'EUR')))`,
			want: []CheckConstraint{c("", "price", "price > 0 AND (currency = 'USD' OR currency = 'EUR')")},
		},
		{
			name: "parenthesis inside string literal does not unbalance",
			ddl:  `CREATE TABLE t (s TEXT CHECK (s <> ')'))`,
			want: []CheckConstraint{c("", "s", "s <> ')'")},
		},
		{
			name: "word check inside a string is ignored",
			ddl:  `CREATE TABLE t (s TEXT DEFAULT 'no check here', x INTEGER)`,
			want: nil,
		},
		{
			name: "CHECK keyword is whole-word bounded (checked not matched)",
			ddl:  `CREATE TABLE t (checked INTEGER, x INTEGER)`,
			want: nil,
		},
		{
			name: "multiple column checks",
			ddl:  `CREATE TABLE t (a INTEGER CHECK (a > 0), b INTEGER CHECK (b < 10))`,
			want: []CheckConstraint{
				c("", "a", "a > 0"),
				c("", "b", "b < 10"),
			},
		},
		{
			name: "quoted column name with check",
			ddl:  `CREATE TABLE t ("my col" INTEGER CHECK ("my col" > 0))`,
			want: []CheckConstraint{c("", "my col", `"my col" > 0`)},
		},
		{
			name: "bracket-quoted column name",
			ddl:  `CREATE TABLE t ([order] INTEGER CHECK ([order] > 0))`,
			want: []CheckConstraint{c("", "order", "[order] > 0")},
		},
		{
			name: "create table as select has no check list",
			ddl:  `CREATE TABLE x AS SELECT count(id) FROM users`,
			want: nil,
		},
		{
			name: "line and block comments ignored",
			ddl: `CREATE TABLE t (
				id INTEGER, -- a comment with ( parens
				/* block ( comment ) */
				x INTEGER CHECK (x > 0))`,
			want: []CheckConstraint{c("", "x", "x > 0")},
		},
		{
			name: "if not exists prefix",
			ddl:  `CREATE TABLE IF NOT EXISTS t (id INTEGER, CHECK (id <> 5))`,
			want: []CheckConstraint{c("", "", "id <> 5")},
		},
		{
			name: "without rowid suffix",
			ddl:  `CREATE TABLE t (id INTEGER PRIMARY KEY, v INTEGER CHECK (v <> 0)) WITHOUT ROWID`,
			want: []CheckConstraint{c("", "v", "v <> 0")},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSQLiteCheckConstraints(tc.ddl)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseSQLiteCheckConstraints()\n got %#v\nwant %#v", got, tc.want)
			}
		})
	}
}

func TestStripCheckWrapper(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"CHECK (a > 0)", "a > 0"},
		{"check (a > 0)", "a > 0"},
		{" CHECK( a > 0 ) ", "a > 0"},
		{"CHECK (a > (0))", "a > (0)"},
		{"FOREIGN KEY (a)", "FOREIGN KEY (a)"}, // not a check → unchanged
		{"CHECK()", ""},
	}
	for _, tc := range tests {
		if got := stripCheckWrapper(tc.in); got != tc.want {
			t.Errorf("stripCheckWrapper(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

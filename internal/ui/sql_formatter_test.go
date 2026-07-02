package ui

import (
	"testing"
)

func TestFormatSQL_SelectFromWhere(t *testing.T) {
	input := "select * from users where id = 1"
	got := formatSQL(input)
	want := "SELECT *\nFROM users\nWHERE id = 1"
	if got != want {
		t.Errorf("formatSQL(%q) =\n%s\nwant:\n%s", input, got, want)
	}
}

func TestFormatSQL_Joins(t *testing.T) {
	input := "select u.name, o.total from users u left join orders o on u.id = o.user_id where o.total > 100"
	got := formatSQL(input)
	want := "SELECT u.name, o.total\nFROM users u\nLEFT JOIN orders o\n    ON u.id = o.user_id\nWHERE o.total > 100"
	if got != want {
		t.Errorf("formatSQL(%q) =\n%s\nwant:\n%s", input, got, want)
	}
}

func TestFormatSQL_GroupOrderLimit(t *testing.T) {
	input := "select name, count(*) from users group by name order by count(*) desc limit 10"
	got := formatSQL(input)
	want := "SELECT name, COUNT(*)\nFROM users\nGROUP BY name\nORDER BY COUNT(*) DESC\nLIMIT 10"
	if got != want {
		t.Errorf("formatSQL(%q) =\n%s\nwant:\n%s", input, got, want)
	}
}

func TestFormatSQL_AndOr(t *testing.T) {
	input := "select * from users where a = 1 and b = 2 or c = 3"
	got := formatSQL(input)
	want := "SELECT *\nFROM users\nWHERE a = 1\n    AND b = 2\n    OR c = 3"
	if got != want {
		t.Errorf("formatSQL(%q) =\n%s\nwant:\n%s", input, got, want)
	}
}

func TestFormatSQL_InsertValues(t *testing.T) {
	input := "insert into users (name, email) values ('foo', 'bar')"
	got := formatSQL(input)
	want := "INSERT INTO users (name, email)\nVALUES ('foo', 'bar')"
	if got != want {
		t.Errorf("formatSQL(%q) =\n%s\nwant:\n%s", input, got, want)
	}
}

func TestFormatSQL_UpdateSet(t *testing.T) {
	input := "update users set name = 'foo' where id = 1"
	got := formatSQL(input)
	want := "UPDATE users\nSET name = 'foo'\nWHERE id = 1"
	if got != want {
		t.Errorf("formatSQL(%q) =\n%s\nwant:\n%s", input, got, want)
	}
}

func TestFormatSQL_DeleteFrom(t *testing.T) {
	input := "delete from users where id = 1"
	got := formatSQL(input)
	want := "DELETE FROM users\nWHERE id = 1"
	if got != want {
		t.Errorf("formatSQL(%q) =\n%s\nwant:\n%s", input, got, want)
	}
}

func TestFormatSQL_MultiStatement(t *testing.T) {
	input := "select * from a; delete from b where id = 1;"
	got := formatSQL(input)
	want := "SELECT *\nFROM a;\nDELETE FROM b\nWHERE id = 1;"
	if got != want {
		t.Errorf("formatSQL(%q) =\n%s\nwant:\n%s", input, got, want)
	}
}

func TestFormatSQL_QualifiedNames(t *testing.T) {
	input := "select u.* from users u"
	got := formatSQL(input)
	want := "SELECT u.*\nFROM users u"
	if got != want {
		t.Errorf("formatSQL(%q) =\n%s\nwant:\n%s", input, got, want)
	}
}

func TestFormatSQL_FunctionCalls(t *testing.T) {
	input := "select count(*), sum(amount) from orders"
	got := formatSQL(input)
	want := "SELECT COUNT(*), SUM(amount)\nFROM orders"
	if got != want {
		t.Errorf("formatSQL(%q) =\n%s\nwant:\n%s", input, got, want)
	}
}

func TestFormatSQL_AlreadyFormatted(t *testing.T) {
	input := "SELECT *\nFROM users\nWHERE id = 1"
	got := formatSQL(input)
	want := "SELECT *\nFROM users\nWHERE id = 1"
	if got != want {
		t.Errorf("formatSQL(%q) =\n%s\nwant:\n%s", input, got, want)
	}
}

func TestFormatSQL_StringWithEscapedQuote(t *testing.T) {
	input := "select * from users where name = 'O''Brien'"
	got := formatSQL(input)
	want := "SELECT *\nFROM users\nWHERE name = 'O''Brien'"
	if got != want {
		t.Errorf("formatSQL(%q) =\n%s\nwant:\n%s", input, got, want)
	}
}

func TestFormatSQL_Empty(t *testing.T) {
	got := formatSQL("")
	if got != "" {
		t.Errorf("formatSQL(\"\") = %q, want empty", got)
	}
}

func TestFormatSQL_Union(t *testing.T) {
	input := "select id from a union select id from b"
	got := formatSQL(input)
	want := "SELECT id\nFROM a\nUNION\nSELECT id\nFROM b"
	if got != want {
		t.Errorf("formatSQL(%q) =\n%s\nwant:\n%s", input, got, want)
	}
}

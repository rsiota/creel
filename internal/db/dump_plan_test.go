package db

import (
	"os/exec"
	"strings"
	"testing"
)

func TestDumpPlanFull(t *testing.T) {
	var p DumpPlan
	if !p.IsFull() || !p.HasContent() {
		t.Fatalf("zero plan should be full with content: %+v", p)
	}
	if len(p.SchemaTables()) != 0 || len(p.DataTables()) != 0 {
		t.Fatal("full plan has no selective tables")
	}
}

func TestDumpPlanSelective(t *testing.T) {
	p := DumpPlan{Tables: []DumpTableSpec{
		{Name: "users", Schema: true, Data: true},
		{Name: "audit", Schema: true, Data: false},
		{Name: "tmp", Schema: false, Data: false},
		{Name: "cache", Schema: false, Data: true},
	}}
	if p.IsFull() {
		t.Fatal("selective plan")
	}
	if !p.HasContent() {
		t.Fatal("expected content")
	}
	schema := p.SchemaTables()
	data := p.DataTables()
	if len(schema) != 2 || schema[0] != "users" || schema[1] != "audit" {
		t.Fatalf("schema = %v", schema)
	}
	if len(data) != 2 || data[0] != "users" || data[1] != "cache" {
		t.Fatalf("data = %v", data)
	}
}

func TestDumpPlanEmpty(t *testing.T) {
	p := DumpPlan{Tables: []DumpTableSpec{{Name: "x", Schema: false, Data: false}}}
	if p.HasContent() {
		t.Fatal("omitted-only plan has no content")
	}
}

func TestMysqlDumpPasses(t *testing.T) {
	full := mysqlDumpPasses(DumpPlan{}, false)
	if len(full) != 1 || full[0].NoData || full[0].NoCreateInfo || len(full[0].Tables) != 0 {
		t.Fatalf("full = %+v", full)
	}
	passes := mysqlDumpPasses(DumpPlan{Tables: []DumpTableSpec{
		{Name: "a", Schema: true, Data: true},
		{Name: "b", Schema: true, Data: false},
	}}, true)
	if len(passes) != 2 {
		t.Fatalf("want 2 passes, got %+v", passes)
	}
	if !passes[0].NoData || !passes[0].Routines || len(passes[0].Tables) != 2 {
		t.Fatalf("schema pass: %+v", passes[0])
	}
	if !passes[1].NoCreateInfo || passes[1].Routines || len(passes[1].Tables) != 1 || passes[1].Tables[0] != "a" {
		t.Fatalf("data pass: %+v", passes[1])
	}
}

func TestBuildMysqlDumpArgsSelective(t *testing.T) {
	cfg := ConnectionConfig{Driver: DriverMySQL, Database: "shop", Host: "db", Port: 3306}
	args := BuildMysqlDumpArgs(cfg, "/tmp/c.cnf", MysqlDumpOptions{
		Tables: []string{"users", "orders"}, NoData: true, Routines: true, Events: true,
	})
	joined := strings.Join(args, " ")
	for _, want := range []string{"--no-data", "--routines", "--events"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %v", want, args)
		}
	}
	if args[len(args)-3] != "shop" || args[len(args)-2] != "users" || args[len(args)-1] != "orders" {
		t.Fatalf("table order: %v", args)
	}
}

func TestBuildPgDumpArgsSelective(t *testing.T) {
	cfg := ConnectionConfig{Driver: DriverPostgres, Database: "shop", Host: "db", Port: 5432}
	args := BuildPgDumpArgs(cfg, PgDumpOptions{Tables: []string{"users"}, SchemaOnly: true})
	joined := strings.Join(args, " ")
	for _, want := range []string{"--schema-only", "--table=users", "--dbname=shop"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %v", want, args)
		}
	}
}

func TestPgDumpPasses(t *testing.T) {
	passes := pgDumpPasses(DumpPlan{Tables: []DumpTableSpec{
		{Name: "a", Schema: true, Data: true},
		{Name: "b", Schema: true, Data: false},
	}})
	if len(passes) != 2 || !passes[0].SchemaOnly || !passes[1].DataOnly {
		t.Fatalf("%+v", passes)
	}
	if len(passes[1].Tables) != 1 || passes[1].Tables[0] != "a" {
		t.Fatalf("data tables: %+v", passes[1])
	}
}

func TestBuildRemoteMysqlDumpScriptSelective(t *testing.T) {
	script := buildRemoteMysqlDumpScript(ConnectionConfig{
		Database: "app", Username: "u", Password: "p", Host: "127.0.0.1", Port: 3306,
	}, DumpPlan{Tables: []DumpTableSpec{
		{Name: "users", Schema: true, Data: true},
		{Name: "audit", Schema: true, Data: false},
	}})
	if !strings.Contains(script, "--no-data") || !strings.Contains(script, "--no-create-info") {
		t.Fatalf("want schema+data passes:\n%s", script)
	}
	if !strings.Contains(script, "'users'") || !strings.Contains(script, "'audit'") {
		t.Fatalf("missing tables:\n%s", script)
	}
}

func TestRunMysqlDumpSelectiveTwoPasses(t *testing.T) {
	var runs [][]string
	restoreRun := SwapRunMysqlDumpCmd(func(cmd *exec.Cmd) error {
		runs = append(runs, append([]string{}, cmd.Args[1:]...))
		_, _ = cmd.Stdout.Write([]byte("-- chunk\n"))
		return nil
	})
	t.Cleanup(restoreRun)
	restoreHelp := SwapRunMysqlDumpHelp(func(string) ([]byte, error) {
		return []byte("no column stats"), nil
	})
	t.Cleanup(restoreHelp)

	out := t.TempDir() + "/out.sql"
	cfg := ConnectionConfig{Driver: DriverMySQL, Database: "shop", Host: "db"}
	plan := DumpPlan{Tables: []DumpTableSpec{
		{Name: "users", Schema: true, Data: true},
		{Name: "logs", Schema: true, Data: false},
	}}
	if err := RunMysqlDump("/usr/bin/mysqldump", cfg, out, nil, plan, nil); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("want 2 runs, got %d: %v", len(runs), runs)
	}
	joined0 := strings.Join(runs[0], " ")
	joined1 := strings.Join(runs[1], " ")
	if !strings.Contains(joined0, "--no-data") || !strings.Contains(joined0, "logs") {
		t.Fatalf("schema pass: %v", runs[0])
	}
	if !strings.Contains(joined1, "--no-create-info") || strings.Contains(joined1, "logs") {
		t.Fatalf("data pass: %v", runs[1])
	}
}

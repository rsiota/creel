package db

import (
	"strings"
	"testing"
	"time"
)

// Live session/DBA coverage for the APIs behind :who, :locks, :kill, :explain
// and :explain!. Catalog tests only smoke these on an idle server; these
// exercise a second backend, a real row-lock wait, terminate, and ANALYZE.

func TestLivePostgresSessionOps(t *testing.T) {
	cfg := livePostgresConfig()
	obs := liveConnect(t, cfg)
	t.Run("who", func(t *testing.T) { liveWho(t, cfg, obs, "postgres") })
	t.Run("explain", func(t *testing.T) { liveExplain(t, obs.DB(), DriverPostgres) })
	t.Run("explain_analyze", func(t *testing.T) { liveExplainAnalyze(t, obs.DB(), DriverPostgres) })
	t.Run("lock_wait", func(t *testing.T) { liveLockWait(t, cfg, DriverPostgres) })
	t.Run("kill", func(t *testing.T) { liveKill(t, cfg, obs, DriverPostgres) })
}

func TestLiveMySQLSessionOps(t *testing.T) {
	cfg := liveMySQLConfig()
	obs := liveConnect(t, cfg)
	t.Run("who", func(t *testing.T) { liveWho(t, cfg, obs, "root") })
	t.Run("explain", func(t *testing.T) { liveExplain(t, obs.DB(), DriverMySQL) })
	t.Run("explain_analyze", func(t *testing.T) { liveExplainAnalyze(t, obs.DB(), DriverMySQL) })
	t.Run("lock_wait", func(t *testing.T) { liveLockWait(t, cfg, DriverMySQL) })
	t.Run("kill", func(t *testing.T) { liveKill(t, cfg, obs, DriverMySQL) })
}

func liveWho(t *testing.T, cfg ConnectionConfig, obs *Connection, wantUser string) {
	t.Helper()
	extra := liveConnect(t, cfg)
	etx, err := extra.DB().Begin(IsolationDefault)
	if err != nil {
		t.Fatalf("extra Begin: %v", err)
	}
	t.Cleanup(func() { _ = etx.Rollback() })
	extraPID := liveBackendPID(t, etx, cfg.Driver)

	sessions, err := obs.DB().Sessions()
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	self, ok := findSelfSession(sessions)
	if !ok {
		t.Fatalf("Sessions() missing Self: %+v", sessions)
	}
	if _, err := parseSessionPID(self.PID); err != nil {
		t.Fatalf("self pid %q: %v", self.PID, err)
	}
	if !strings.EqualFold(self.User, wantUser) {
		t.Fatalf("self user = %q, want %q", self.User, wantUser)
	}
	if self.DB == "" {
		t.Fatalf("self DB empty: %+v", self)
	}
	if self.State == "" {
		t.Fatalf("self State empty: %+v", self)
	}

	extraS, ok := findSession(sessions, extraPID)
	if !ok {
		t.Fatalf("Sessions() missing extra pid %s: %+v", extraPID, sessions)
	}
	if extraS.Self {
		t.Fatalf("extra pid %s marked Self", extraPID)
	}
}

func liveExplain(t *testing.T, d DB, driver Driver) {
	t.Helper()
	table := "creel_live_sess_plan"
	liveResetPlanTable(t, d, driver, table)
	qtable := table
	if driver == DriverPostgres {
		qtable = "public." + table
	}

	full, err := d.Execute("EXPLAIN SELECT * FROM " + qtable)
	if err != nil {
		t.Fatalf("EXPLAIN full scan: %v", err)
	}
	if len(full.Rows) == 0 {
		t.Fatal("EXPLAIN returned no rows")
	}
	findings := DiagnoseExplain(driver, full, d.Indexes)
	if !hasFindingIssue(findings, "Sequential scan") && !hasFindingIssue(findings, "Full table scan") {
		t.Fatalf("DiagnoseExplain(full scan) = %+v, plan:\n%s", findings, planText(full))
	}

	pk, err := d.Execute("EXPLAIN SELECT * FROM " + qtable + " WHERE id = 1")
	if err != nil {
		t.Fatalf("EXPLAIN pk: %v", err)
	}
	if len(pk.Rows) == 0 {
		t.Fatal("EXPLAIN pk returned no rows")
	}
	pkFindings := DiagnoseExplain(driver, pk, d.Indexes)
	if len(pkFindings) == 0 {
		t.Fatal("DiagnoseExplain(pk) returned nothing")
	}
}

func liveExplainAnalyze(t *testing.T, d DB, driver Driver) {
	t.Helper()
	table := "creel_live_sess_plan"
	liveResetPlanTable(t, d, driver, table)
	qtable := table
	if driver == DriverPostgres {
		qtable = "public." + table
	}

	plan, err := d.Execute("EXPLAIN ANALYZE SELECT * FROM " + qtable + " WHERE id = 1")
	if err != nil {
		t.Fatalf("EXPLAIN ANALYZE: %v", err)
	}
	text := strings.ToLower(planText(plan))
	if !strings.Contains(text, "actual time") && !strings.Contains(text, "execution time") {
		t.Fatalf("EXPLAIN ANALYZE missing timing, plan:\n%s", planText(plan))
	}
	if findings := DiagnoseExplain(driver, plan, d.Indexes); len(findings) == 0 {
		t.Fatal("DiagnoseExplain(ANALYZE) returned nothing")
	}
}

func liveLockWait(t *testing.T, cfg ConnectionConfig, driver Driver) {
	t.Helper()
	obs := liveConnect(t, cfg)
	blocker := liveConnect(t, cfg)
	waiter := liveConnect(t, cfg)
	d := obs.DB()

	table := "creel_live_sess_lock"
	mustExec(t, d, "DROP TABLE IF EXISTS "+table)
	switch driver {
	case DriverPostgres:
		mustExec(t, d, `CREATE TABLE `+table+` (id INTEGER PRIMARY KEY, n INTEGER NOT NULL)`)
	default:
		mustExec(t, d, `CREATE TABLE `+table+` (id INT PRIMARY KEY, n INT NOT NULL) ENGINE=InnoDB`)
	}
	mustExec(t, d, `INSERT INTO `+table+` (id, n) VALUES (1, 0)`)
	t.Cleanup(func() { _, _ = d.Exec("DROP TABLE IF EXISTS " + table) })

	btx, err := blocker.DB().Begin(IsolationDefault)
	if err != nil {
		t.Fatalf("blocker Begin: %v", err)
	}
	t.Cleanup(func() { _ = btx.Rollback() })
	if _, err := btx.Exec("UPDATE " + table + " SET n = 1 WHERE id = 1"); err != nil {
		t.Fatalf("blocker UPDATE: %v", err)
	}
	blockerPID := liveBackendPID(t, btx, driver)

	wtx, err := waiter.DB().Begin(IsolationDefault)
	if err != nil {
		t.Fatalf("waiter Begin: %v", err)
	}
	t.Cleanup(func() { _ = wtx.Rollback() })
	if driver == DriverPostgres {
		if _, err := wtx.Exec("SET LOCAL lock_timeout = '8s'"); err != nil {
			t.Fatalf("SET lock_timeout: %v", err)
		}
	} else if _, err := wtx.Exec("SET innodb_lock_wait_timeout = 8"); err != nil {
		t.Fatalf("SET innodb_lock_wait_timeout: %v", err)
	}
	waiterPID := liveBackendPID(t, wtx, driver)

	errc := make(chan error, 1)
	go func() {
		_, err := wtx.Exec("UPDATE " + table + " SET n = 2 WHERE id = 1")
		errc <- err
	}()

	wait := waitLockPair(t, d, waiterPID, blockerPID, 5*time.Second)
	if rel := strings.ToLower(wait.Relation); rel != "" && !strings.Contains(rel, table) {
		t.Fatalf("lock relation = %q, want %s", wait.Relation, table)
	}

	if err := btx.Rollback(); err != nil {
		t.Fatalf("blocker Rollback: %v", err)
	}
	select {
	case <-errc:
	case <-time.After(6 * time.Second):
		t.Fatal("waiter still blocked after blocker rollback")
	}
}

func liveKill(t *testing.T, cfg ConnectionConfig, obs *Connection, driver Driver) {
	t.Helper()
	extra := liveConnect(t, cfg)
	etx, err := extra.DB().Begin(IsolationDefault)
	if err != nil {
		t.Fatalf("extra Begin: %v", err)
	}
	pid := liveBackendPID(t, etx, driver)

	if err := obs.DB().KillSession(pid); err != nil {
		t.Fatalf("KillSession(%s): %v", pid, err)
	}
	if _, err := etx.Execute("SELECT 1"); err == nil {
		t.Fatal("killed session still executes")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		sessions, err := obs.DB().Sessions()
		if err != nil {
			t.Fatalf("Sessions after kill: %v", err)
		}
		if _, ok := findSession(sessions, pid); !ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("Sessions() still lists killed pid %s", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func liveResetPlanTable(t *testing.T, d DB, driver Driver, table string) {
	t.Helper()
	mustExec(t, d, "DROP TABLE IF EXISTS "+table)
	switch driver {
	case DriverPostgres:
		mustExec(t, d, `CREATE TABLE `+table+` (
			id INTEGER PRIMARY KEY,
			note TEXT NOT NULL
		)`)
	default:
		mustExec(t, d, `CREATE TABLE `+table+` (
			id INT PRIMARY KEY,
			note VARCHAR(32) NOT NULL
		) ENGINE=InnoDB`)
	}
	mustExec(t, d, `INSERT INTO `+table+` (id, note) VALUES (1, 'a'), (2, 'b'), (3, 'c')`)
	t.Cleanup(func() { _, _ = d.Exec("DROP TABLE IF EXISTS " + table) })
}

type liveExec interface {
	Execute(query string) (Result, error)
}

func liveBackendPID(t *testing.T, exec liveExec, driver Driver) string {
	t.Helper()
	q := "SELECT pg_backend_pid()"
	if driver == DriverMySQL {
		q = "SELECT CONNECTION_ID()"
	}
	res, err := exec.Execute(q)
	if err != nil {
		t.Fatalf("backend pid: %v", err)
	}
	pid := firstCell(t, res)
	if _, err := parseSessionPID(pid); err != nil {
		t.Fatalf("backend pid %q: %v", pid, err)
	}
	return pid
}

func firstCell(t *testing.T, r Result) string {
	t.Helper()
	if len(r.Rows) == 0 || len(r.Rows[0]) == 0 {
		t.Fatal("empty result")
	}
	return strings.TrimSpace(r.Rows[0][0])
}

func findSelfSession(sessions []SessionInfo) (SessionInfo, bool) {
	for _, s := range sessions {
		if s.Self {
			return s, true
		}
	}
	return SessionInfo{}, false
}

func findSession(sessions []SessionInfo, pid string) (SessionInfo, bool) {
	for _, s := range sessions {
		if s.PID == pid {
			return s, true
		}
	}
	return SessionInfo{}, false
}

func hasFindingIssue(findings []DiagnosisFinding, issue string) bool {
	for _, f := range findings {
		if f.Issue == issue {
			return true
		}
	}
	return false
}

func planText(r Result) string {
	var b strings.Builder
	for _, row := range r.Rows {
		b.WriteString(strings.Join(row, " "))
		b.WriteByte('\n')
	}
	return b.String()
}

func waitLockPair(t *testing.T, d DB, waitingPID, blockingPID string, dmax time.Duration) LockWait {
	t.Helper()
	deadline := time.Now().Add(dmax)
	var last []LockWait
	for time.Now().Before(deadline) {
		waits, err := d.Locks()
		if err != nil {
			t.Fatalf("Locks: %v", err)
		}
		last = waits
		for _, w := range waits {
			if w.WaitingPID == waitingPID && w.BlockingPID == blockingPID {
				return w
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no lock wait %s blocked by %s; last Locks()=%+v", waitingPID, blockingPID, last)
	return LockWait{}
}

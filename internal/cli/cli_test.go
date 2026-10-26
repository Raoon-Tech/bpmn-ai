package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// runCLI executes a CLI command with the given args and sqlite env, capturing stdout/stderr.
func runCLI(t *testing.T, dbPath string, args ...string) (int, string, string) {
	t.Helper()
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_FILE", dbPath)
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_AUTO_MIGRATE", "true")
	t.Setenv("SERVER_DISABLE_CSRF", "true")

	// capture stdout
	oldOut := os.Stdout
	oldErr := os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	code := Run(args)

	wOut.Close()
	wErr.Close()
	os.Stdout = oldOut
	os.Stderr = oldErr

	var bufOut, bufErr bytes.Buffer
	bufOut.ReadFrom(rOut)
	bufErr.ReadFrom(rErr)
	return code, bufOut.String(), bufErr.String()
}

func TestVersion(t *testing.T) {
	db := filepath.Join(t.TempDir(), "v.db")
	code, out, _ := runCLI(t, db, "version")
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if !contains(out, "bpmn-ai") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestConfigShow(t *testing.T) {
	db := filepath.Join(t.TempDir(), "c.db")
	code, out, _ := runCLI(t, db, "config", "show")
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if !contains(out, "database.driver") {
		t.Errorf("missing database.driver in output: %s", out)
	}
}

func TestDoctor(t *testing.T) {
	db := filepath.Join(t.TempDir(), "d.db")
	code, out, _ := runCLI(t, db, "doctor")
	// doctor may return 0 or 1 depending on port availability; just ensure it runs
	_ = code
	if !contains(out, "configuration") {
		t.Errorf("unexpected doctor output: %s", out)
	}
}

func TestMigrateStatus(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	code, out, _ := runCLI(t, db, "migrate", "status")
	if code != 0 {
		t.Fatalf("expected 0, got %d: %s", code, out)
	}
	if !contains(out, "0001_initial_schema") {
		t.Errorf("expected migration listed: %s", out)
	}
}

func TestMigrateUp(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m2.db")
	code, _, errOut := runCLI(t, db, "migrate", "up")
	if code != 0 {
		t.Fatalf("expected 0, got %d: %s", code, errOut)
	}
}

func TestMigrateCreate(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m3.db")
	dir := filepath.Join(t.TempDir(), "migs")
	code, out, _ := runCLI(t, db, "migrate", "create", "--dir", dir, "test_migration")
	if code != 0 {
		t.Fatalf("expected 0, got %d: %s", code, out)
	}
	if !contains(out, "created") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestDBPing(t *testing.T) {
	db := filepath.Join(t.TempDir(), "p.db")
	code, out, _ := runCLI(t, db, "db", "ping")
	if code != 0 {
		t.Fatalf("expected 0, got %d: %s", code, out)
	}
	if !contains(out, "ok:") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestDBStats(t *testing.T) {
	db := filepath.Join(t.TempDir(), "s.db")
	code, out, _ := runCLI(t, db, "db", "stats")
	if code != 0 {
		t.Fatalf("expected 0, got %d: %s", code, out)
	}
	if !contains(out, "processes") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestDBSeed(t *testing.T) {
	db := filepath.Join(t.TempDir(), "seed.db")
	code, out, _ := runCLI(t, db, "db", "seed", "../../testdata/simple_sequence.bpmn")
	if code != 0 {
		t.Fatalf("expected 0, got %d: %s", code, out)
	}
	if !contains(out, "stored in") && !contains(out, "deployed") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestProcessList(t *testing.T) {
	db := filepath.Join(t.TempDir(), "pl.db")
	runCLI(t, db, "db", "seed", "../../testdata/simple_sequence.bpmn")
	code, out, _ := runCLI(t, db, "process", "list")
	if code != 0 {
		t.Fatalf("expected 0, got %d: %s", code, out)
	}
	if !contains(out, "simple-sequence") {
		t.Errorf("expected process in list: %s", out)
	}
}

func TestProcessGet(t *testing.T) {
	db := filepath.Join(t.TempDir(), "pg.db")
	runCLI(t, db, "db", "seed", "../../testdata/simple_sequence.bpmn")
	code, out, _ := runCLI(t, db, "process", "get", "simple-sequence")
	if code != 0 {
		t.Fatalf("expected 0, got %d: %s", code, out)
	}
	if !contains(out, "Simple Sequence Process") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestProcessDelete(t *testing.T) {
	db := filepath.Join(t.TempDir(), "pd.db")
	runCLI(t, db, "db", "seed", "../../testdata/simple_sequence.bpmn")
	code, _, _ := runCLI(t, db, "process", "delete", "simple-sequence", "--force")
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	code, out, _ := runCLI(t, db, "process", "get", "simple-sequence")
	if code == 0 {
		t.Errorf("expected error after delete, got success: %s", out)
	}
}

func TestCaseList(t *testing.T) {
	db := filepath.Join(t.TempDir(), "cl.db")
	code, out, _ := runCLI(t, db, "case", "list")
	if code != 0 {
		t.Fatalf("expected 0, got %d: %s", code, out)
	}
}

func TestTaskList(t *testing.T) {
	db := filepath.Join(t.TempDir(), "tl.db")
	code, out, _ := runCLI(t, db, "task", "list", "00000000-0000-0000-0000-000000000000")
	// may error (case not found) but command should run
	_ = code
	_ = out
}

func TestJobList(t *testing.T) {
	db := filepath.Join(t.TempDir(), "jl.db")
	code, out, _ := runCLI(t, db, "job", "list")
	if code != 0 {
		t.Fatalf("expected 0, got %d: %s", code, out)
	}
}

func TestJobDrain(t *testing.T) {
	db := filepath.Join(t.TempDir(), "jd.db")
	code, out, _ := runCLI(t, db, "job", "drain", "--limit", "5")
	if code != 0 {
		t.Fatalf("expected 0, got %d: %s", code, out)
	}
	if !contains(out, "processed") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestAIProviderList(t *testing.T) {
	db := filepath.Join(t.TempDir(), "ai.db")
	code, out, _ := runCLI(t, db, "ai", "provider", "list")
	if code != 0 {
		t.Fatalf("expected 0, got %d: %s", code, out)
	}
}

func TestAIProviderAddShowRemove(t *testing.T) {
	db := filepath.Join(t.TempDir(), "aip.db")
	code, out, _ := runCLI(t, db, "ai", "provider", "add", "openai", "--type", "openai", "--key", "sk-test", "--default")
	if code != 0 {
		t.Fatalf("add: expected 0, got %d: %s", code, out)
	}
	code, out, _ = runCLI(t, db, "ai", "provider", "list")
	if code != 0 || !contains(out, "openai") {
		t.Fatalf("list: %d %s", code, out)
	}
	code, out, _ = runCLI(t, db, "ai", "provider", "show", "openai")
	if code != 0 || !contains(out, "openai") {
		t.Fatalf("show: %d %s", code, out)
	}
	code, _, _ = runCLI(t, db, "ai", "provider", "remove", "openai")
	if code != 0 {
		t.Fatalf("remove: expected 0, got %d", code)
	}
}

func TestAIProviderKeyStdin(t *testing.T) {
	db := filepath.Join(t.TempDir(), "aik.db")
	// runCLI doesn't support stdin; test add with --key instead
	code, out, _ := runCLI(t, db, "ai", "provider", "add", "anthropic", "--type", "anthropic", "--key", "sk-ant")
	if code != 0 {
		t.Fatalf("expected 0, got %d: %s", code, out)
	}
}

func TestAIProfileList(t *testing.T) {
	db := filepath.Join(t.TempDir(), "apl.db")
	code, out, _ := runCLI(t, db, "ai", "profile", "list")
	if code != 0 {
		t.Fatalf("expected 0, got %d: %s", code, out)
	}
}

func TestAIProfileSetGetReset(t *testing.T) {
	db := filepath.Join(t.TempDir(), "apr.db")
	code, out, _ := runCLI(t, db, "ai", "profile", "set", "fast", "--model", "gpt-4o-mini", "--max-tokens", "1024", "--priority", "speed")
	if code != 0 {
		t.Fatalf("set: expected 0, got %d: %s", code, out)
	}
	code, out, _ = runCLI(t, db, "ai", "profile", "get", "fast")
	if code != 0 || !contains(out, "gpt-4o-mini") {
		t.Fatalf("get: %d %s", code, out)
	}
	code, out, _ = runCLI(t, db, "ai", "profile", "reset", "--name", "fast")
	if code != 0 {
		t.Fatalf("reset: expected 0, got %d: %s", code, out)
	}
}

func TestUnknownCommand(t *testing.T) {
	db := filepath.Join(t.TempDir(), "uc.db")
	code, _, _ := runCLI(t, db, "nonexistent")
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}

func TestNoArgs(t *testing.T) {
	// Run with no args starts serve; skip (would block). Test usage via help.
	db := filepath.Join(t.TempDir(), "na.db")
	code, out, _ := runCLI(t, db, "help")
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if !contains(out, "COMMANDS") {
		t.Errorf("unexpected help output: %s", out)
	}
}

func TestCaseGet(t *testing.T) {
	db := filepath.Join(t.TempDir(), "cg.db")
	runCLI(t, db, "db", "seed", "../../testdata/simple_sequence.bpmn")
	// start a case first via direct... use case start
	code, out, _ := runCLI(t, db, "case", "start", "simple-sequence")
	if code != 0 {
		t.Fatalf("start: %d %s", code, out)
	}
	// extract case id
	cid := ""
	for _, line := range splitLines(out) {
		if len(line) > 8 && line[:8] == "case_id:" {
			cid = line[9:]
		}
	}
	if cid == "" {
		t.Fatal("no case id")
	}
	code, out, _ = runCLI(t, db, "case", "get", cid)
	if code != 0 {
		t.Fatalf("get: %d %s", code, out)
	}
	if !contains(out, cid) {
		t.Errorf("expected case id in output: %s", out)
	}
}

func TestCaseHistory(t *testing.T) {
	db := filepath.Join(t.TempDir(), "ch.db")
	runCLI(t, db, "db", "seed", "../../testdata/simple_sequence.bpmn")
	code, out, _ := runCLI(t, db, "case", "start", "simple-sequence")
	if code != 0 {
		t.Fatalf("start: %d %s", code, out)
	}
	cid := extractCaseID(out)
	code, out, _ = runCLI(t, db, "case", "history", cid)
	if code != 0 {
		t.Fatalf("history: %d %s", code, out)
	}
}

func TestCaseCancel(t *testing.T) {
	db := filepath.Join(t.TempDir(), "cc.db")
	runCLI(t, db, "db", "seed", "../../testdata/simple_sequence.bpmn")
	code, out, _ := runCLI(t, db, "case", "start", "simple-sequence")
	if code != 0 {
		t.Fatalf("start: %d %s", code, out)
	}
	cid := extractCaseID(out)
	code, out, _ = runCLI(t, db, "case", "cancel", cid)
	if code != 0 {
		t.Fatalf("cancel: %d %s", code, out)
	}
	if !contains(out, "terminated") {
		t.Errorf("unexpected: %s", out)
	}
}

func TestJobRetry(t *testing.T) {
	db := filepath.Join(t.TempDir(), "jr.db")
	// create a job via db seed won't create jobs; use a process with service task? skip if none
	code, _, _ := runCLI(t, db, "job", "retry", "00000000-0000-0000-0000-000000000000")
	// expect error (not found) but command runs
	if code == 0 {
		t.Error("expected non-zero for missing job")
	}
}

func TestMessageSend(t *testing.T) {
	db := filepath.Join(t.TempDir(), "ms.db")
	code, _, _ := runCLI(t, db, "message", "send", "00000000-0000-0000-0000-000000000000", "msg-ref")
	if code == 0 {
		t.Error("expected non-zero for missing instance")
	}
}

func TestSignalSend(t *testing.T) {
	db := filepath.Join(t.TempDir(), "ss.db")
	code, out, _ := runCLI(t, db, "signal", "send", "sig-ref")
	// With no waiting SignalCatch, the command reports 0 affected instances (success).
	_ = code
	_ = out
}

func TestMigrateDown(t *testing.T) {
	db := filepath.Join(t.TempDir(), "md.db")
	runCLI(t, db, "migrate", "up")
	code, out, _ := runCLI(t, db, "migrate", "down", "--steps", "1", "--force")
	if code != 0 {
		t.Fatalf("expected 0, got %d: %s", code, out)
	}
}

func TestMigrateAuto(t *testing.T) {
	db := filepath.Join(t.TempDir(), "ma.db")
	code, out, _ := runCLI(t, db, "migrate", "auto")
	if code != 0 {
		t.Fatalf("expected 0, got %d: %s", code, out)
	}
	if !contains(out, "synchronized") {
		t.Errorf("unexpected: %s", out)
	}
}

func TestMigrateBaseline(t *testing.T) {
	db := filepath.Join(t.TempDir(), "mb.db")
	// baseline all registered migrations
	code, out, _ := runCLI(t, db, "migrate", "baseline", "--all")
	if code != 0 {
		t.Fatalf("expected 0, got %d: %s", code, out)
	}
}

func TestMigrateReset(t *testing.T) {
	db := filepath.Join(t.TempDir(), "mr.db")
	runCLI(t, db, "migrate", "up")
	code, out, _ := runCLI(t, db, "migrate", "reset", "--force")
	if code != 0 {
		t.Fatalf("expected 0, got %d: %s", code, out)
	}
}

func TestAIProviderSetDefault(t *testing.T) {
	db := filepath.Join(t.TempDir(), "aisd.db")
	runCLI(t, db, "ai", "provider", "add", "p1", "--type", "openai", "--key", "k1")
	runCLI(t, db, "ai", "provider", "add", "p2", "--type", "openai", "--key", "k2")
	code, out, _ := runCLI(t, db, "ai", "provider", "set-default", "p2")
	if code != 0 {
		t.Fatalf("expected 0, got %d: %s", code, out)
	}
	if !contains(out, "marked as default") {
		t.Errorf("unexpected: %s", out)
	}
}

func TestConfigShowJSON(t *testing.T) {
	db := filepath.Join(t.TempDir(), "csj.db")
	code, out, _ := runCLI(t, db, "config", "show", "--json")
	if code != 0 {
		t.Fatalf("expected 0, got %d: %s", code, out)
	}
	if !contains(out, "Database") && !contains(out, "database") {
		t.Errorf("unexpected: %s", out)
	}
}

func extractCaseID(out string) string {
	for _, line := range splitLines(out) {
		if len(line) > 8 && line[:8] == "case_id:" {
			return line[9:]
		}
	}
	return ""
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func contains(s, substr string) bool {
	return len(s) > 0 && (len(substr) == 0 || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

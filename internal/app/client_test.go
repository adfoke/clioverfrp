package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/adfoke/clioverfrp/internal/config"
	"github.com/adfoke/clioverfrp/internal/protocol"
)

func TestLoadTasksSkipsBlankLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.jsonl")
	content := "\n" +
		`{"type":"exec","cmd":"uptime"}` + "\n" +
		"\n" +
		`{"type":"ls","path":"/tmp"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	tasks, err := LoadTasks(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("unexpected task count: %d", len(tasks))
	}
	if tasks[0].Type != "exec" || tasks[1].Type != "ls" {
		t.Fatalf("unexpected tasks: %+v", tasks)
	}
}

func TestLoadTasksRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"exec"`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadTasks(path); err == nil {
		t.Fatal("expected invalid jsonl error")
	}
}

func TestWriteTaskReportAndCleanup(t *testing.T) {
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "task_report.json")

	results := []protocol.Result{
		{Success: true, Type: "exec", ExitCode: ExitOK},
		{Success: false, Type: "pull", ExitCode: ExitTransfer, Error: "broken"},
	}
	if err := WriteTaskReport(reportPath, results); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var report protocol.TaskReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if report.Success {
		t.Fatal("expected failed report")
	}
	if report.Count != 2 {
		t.Fatalf("unexpected report count: %d", report.Count)
	}

	tempA := filepath.Join(dir, "a"+config.DefaultTempSuffix)
	tempB := filepath.Join(dir, "b"+config.DefaultTempSuffix+".meta.json")
	keep := filepath.Join(dir, "keep.txt")
	for _, path := range []string{tempA, tempB, keep} {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	res, code := Cleanup(dir, config.DefaultTempSuffix)
	if code != ExitOK || !res.Success {
		t.Fatalf("unexpected cleanup result: code=%d result=%+v", code, res)
	}
	if _, err := os.Stat(tempA); !os.IsNotExist(err) {
		t.Fatalf("expected temp file removed, err=%v", err)
	}
	if _, err := os.Stat(tempB); !os.IsNotExist(err) {
		t.Fatalf("expected meta file removed, err=%v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("expected keep file preserved: %v", err)
	}
}

package ragent

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adfoke/clioverfrp/internal/app"
	"github.com/adfoke/clioverfrp/internal/config"
	"github.com/adfoke/clioverfrp/internal/fileutil"
	"github.com/adfoke/clioverfrp/internal/protocol"
)

func TestClientServerExecListPushPull(t *testing.T) {
	client := newTestClient(t)
	dir := t.TempDir()
	localPath := filepath.Join(dir, "local.txt")
	remotePath := filepath.Join(dir, "remote.txt")
	downloadPath := filepath.Join(dir, "download.txt")

	if err := os.WriteFile(localPath, []byte("hello from push"), 0o644); err != nil {
		t.Fatal(err)
	}

	execResult, code := client.Exec("printf e2e-ok", false)
	if code != app.ExitOK || !execResult.Success {
		t.Fatalf("exec failed: code=%d result=%+v", code, execResult)
	}
	output, ok := execResult.Output.(map[string]any)
	if !ok || output["stdout"] != "e2e-ok" {
		t.Fatalf("unexpected exec output: %#v", execResult.Output)
	}

	listResult, code := client.List(dir)
	if code != app.ExitOK || !listResult.Success {
		t.Fatalf("ls failed: code=%d result=%+v", code, listResult)
	}
	entries, ok := listResult.Output.([]protocol.DirEntry)
	if !ok || len(entries) == 0 {
		t.Fatalf("unexpected ls output: %#v", listResult.Output)
	}

	pushResult, code := client.Push(localPath, remotePath, app.TransferOptions{})
	if code != app.ExitOK || !pushResult.Success {
		t.Fatalf("push failed: code=%d result=%+v", code, pushResult)
	}
	remoteData, err := os.ReadFile(remotePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(remoteData) != "hello from push" {
		t.Fatalf("unexpected remote content: %q", string(remoteData))
	}

	pullResult, code := client.Pull(remotePath, downloadPath, app.TransferOptions{})
	if code != app.ExitOK || !pullResult.Success {
		t.Fatalf("pull failed: code=%d result=%+v", code, pullResult)
	}
	downloadData, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(downloadData) != "hello from push" {
		t.Fatalf("unexpected download content: %q", string(downloadData))
	}

	info, err := client.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.OS == "" || info.Arch == "" {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestPushResumesFromExistingRemoteTemp(t *testing.T) {
	client := newTestClient(t)
	dir := t.TempDir()
	localPath := filepath.Join(dir, "resume-src.txt")
	remotePath := filepath.Join(dir, "resume-remote.txt")
	content := []byte("resume me completely")

	if err := os.WriteFile(localPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := fileutil.SHA256File(localPath)
	if err != nil {
		t.Fatal(err)
	}

	tempPath := remotePath + config.DefaultTempSuffix
	if err := os.WriteFile(tempPath, content[:7], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteMeta(fileutil.MetaPath(tempPath), fileutil.ResumeMeta{
		TargetPath: remotePath,
		Size:       int64(len(content)),
		SHA256:     sum,
	}); err != nil {
		t.Fatal(err)
	}

	result, code := client.Push(localPath, remotePath, app.TransferOptions{})
	if code != app.ExitOK || !result.Success {
		t.Fatalf("push resume failed: code=%d result=%+v", code, result)
	}
	data, err := os.ReadFile(remotePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(content) {
		t.Fatalf("unexpected remote content: %q", string(data))
	}
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Fatalf("expected temp file removed, err=%v", err)
	}
	if _, err := os.Stat(fileutil.MetaPath(tempPath)); !os.IsNotExist(err) {
		t.Fatalf("expected meta file removed, err=%v", err)
	}
}

func TestPullResumesFromExistingLocalTemp(t *testing.T) {
	client := newTestClient(t)
	dir := t.TempDir()
	remotePath := filepath.Join(dir, "pull-remote.txt")
	localPath := filepath.Join(dir, "pull-local.txt")
	content := []byte("resume pull content")

	if err := os.WriteFile(remotePath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := fileutil.SHA256File(remotePath)
	if err != nil {
		t.Fatal(err)
	}

	tempPath := localPath + config.DefaultTempSuffix
	if err := os.WriteFile(tempPath, content[:5], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteMeta(fileutil.MetaPath(tempPath), fileutil.ResumeMeta{
		TargetPath: remotePath,
		Size:       int64(len(content)),
		SHA256:     sum,
	}); err != nil {
		t.Fatal(err)
	}

	result, code := client.Pull(remotePath, localPath, app.TransferOptions{})
	if code != app.ExitOK || !result.Success {
		t.Fatalf("pull resume failed: code=%d result=%+v", code, result)
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(content) {
		t.Fatalf("unexpected local content: %q", string(data))
	}
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Fatalf("expected temp file removed, err=%v", err)
	}
	if _, err := os.Stat(fileutil.MetaPath(tempPath)); !os.IsNotExist(err) {
		t.Fatalf("expected meta file removed, err=%v", err)
	}
}

func TestPullDiscardsStaleResumeStateWhenRemoteChanged(t *testing.T) {
	client := newTestClient(t)
	dir := t.TempDir()
	remotePath := filepath.Join(dir, "changed-remote.txt")
	localPath := filepath.Join(dir, "changed-local.txt")
	content := []byte("fresh remote content")

	if err := os.WriteFile(remotePath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	tempPath := localPath + config.DefaultTempSuffix
	if err := os.WriteFile(tempPath, []byte("stale-partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteMeta(fileutil.MetaPath(tempPath), fileutil.ResumeMeta{
		TargetPath: remotePath,
		Size:       999,
		SHA256:     "stale-hash",
	}); err != nil {
		t.Fatal(err)
	}

	result, code := client.Pull(remotePath, localPath, app.TransferOptions{})
	if code != app.ExitOK || !result.Success {
		t.Fatalf("pull with stale resume state failed: code=%d result=%+v", code, result)
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(content) {
		t.Fatalf("unexpected local content: %q", string(data))
	}
}

func TestPushTruncatesStaleTempWithoutMeta(t *testing.T) {
	client := newTestClient(t)
	dir := t.TempDir()
	localPath := filepath.Join(dir, "push-src.txt")
	remotePath := filepath.Join(dir, "push-dst.txt")
	content := []byte("short file")

	if err := os.WriteFile(localPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	tempPath := remotePath + config.DefaultTempSuffix
	if err := os.WriteFile(tempPath, []byte("this stale temp file is longer than the upload"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, code := client.Push(localPath, remotePath, app.TransferOptions{})
	if code != app.ExitOK || !result.Success {
		t.Fatalf("push with stale temp failed: code=%d result=%+v", code, result)
	}
	data, err := os.ReadFile(remotePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(content) {
		t.Fatalf("unexpected remote content: %q", string(data))
	}
}

func newTestClient(t *testing.T) *app.Client {
	t.Helper()

	server := &Server{
		Token:      "test-token",
		ChunkSize:  4,
		TempSuffix: config.DefaultTempSuffix,
	}
	httpServer := httptest.NewServer(http.HandlerFunc(server.handleWS))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	return app.NewClient(config.Config{
		WSURL:            wsURL,
		Token:            "test-token",
		ResumeEnabled:    true,
		ResumeTempSuffix: config.DefaultTempSuffix,
		ChunkSize:        4,
	})
}

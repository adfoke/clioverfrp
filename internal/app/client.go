package app

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/adfoke/clioverfrp/internal/config"
	"github.com/adfoke/clioverfrp/internal/fileutil"
	"github.com/adfoke/clioverfrp/internal/protocol"
	"github.com/adfoke/clioverfrp/internal/wsjson"
)

const (
	ExitOK       = 0
	ExitConn     = 1
	ExitTransfer = 2
	ExitCommand  = 3
)

type Client struct {
	Config config.Config
}

type TransferOptions struct {
	Force        bool
	SkipExisting bool
}

func (c *Client) Info() (protocol.AgentRuntimeInfo, error) {
	conn, _, err := c.connect()
	if err != nil {
		return protocol.AgentRuntimeInfo{}, err
	}
	defer conn.Close()

	if err := wsjson.Write(conn, protocol.Message{Type: "info_request"}); err != nil {
		return protocol.AgentRuntimeInfo{}, err
	}
	msg, err := wsjson.Read(conn)
	if err != nil {
		return protocol.AgentRuntimeInfo{}, err
	}
	if msg.Type == "error" {
		return protocol.AgentRuntimeInfo{}, errors.New(msg.Error)
	}
	if msg.Type != "info_result" {
		return protocol.AgentRuntimeInfo{}, errors.New("unexpected server response")
	}
	return protocol.AgentRuntimeInfo{
		Hostname: msg.Hostname,
		OS:       msg.OS,
		Arch:     msg.Arch,
	}, nil
}

func NewClient(cfg config.Config) *Client {
	return &Client{Config: cfg}
}

func (c *Client) Exec(command string, liveOutput bool) (protocol.Result, int) {
	start := time.Now()
	result := protocol.Result{Type: "exec"}
	conn, exitCode, err := c.connect()
	if err != nil {
		result.Error = err.Error()
		result.DurationMS = time.Since(start).Milliseconds()
		result.ExitCode = ExitConn
		return result, exitCode
	}
	defer conn.Close()

	if err := wsjson.Write(conn, protocol.Message{Type: "exec_request", Cmd: command}); err != nil {
		result.Error = err.Error()
		result.DurationMS = time.Since(start).Milliseconds()
		result.ExitCode = ExitConn
		return result, ExitConn
	}

	var stdout strings.Builder
	var stderr strings.Builder
	for {
		msg, err := wsjson.Read(conn)
		if err != nil {
			result.Error = err.Error()
			result.DurationMS = time.Since(start).Milliseconds()
			result.ExitCode = ExitConn
			return result, ExitConn
		}
		switch msg.Type {
		case "exec_stream":
			if msg.Stream == "stderr" {
				stderr.WriteString(msg.Data)
				if liveOutput {
					_, _ = fmt.Fprint(os.Stderr, msg.Data)
				}
			} else {
				stdout.WriteString(msg.Data)
				if liveOutput {
					_, _ = fmt.Fprint(os.Stdout, msg.Data)
				}
			}
		case "exec_result":
			result.Success = msg.Success
			result.DurationMS = time.Since(start).Milliseconds()
			result.ExitCode = msg.ExitCode
			result.Output = map[string]any{
				"stdout": stdout.String(),
				"stderr": stderr.String(),
			}
			if !msg.Success {
				result.Error = strings.TrimSpace(stderr.String())
				if result.Error == "" {
					result.Error = "command failed"
				}
				return result, ExitCommand
			}
			return result, ExitOK
		case "error":
			result.Error = msg.Error
			result.DurationMS = time.Since(start).Milliseconds()
			result.ExitCode = ExitConn
			return result, ExitConn
		}
	}
}

func (c *Client) List(path string) (protocol.Result, int) {
	start := time.Now()
	result := protocol.Result{Type: "ls"}
	conn, exitCode, err := c.connect()
	if err != nil {
		result.Error = err.Error()
		result.DurationMS = time.Since(start).Milliseconds()
		result.ExitCode = ExitConn
		return result, exitCode
	}
	defer conn.Close()

	if err := wsjson.Write(conn, protocol.Message{Type: "ls_request", Path: path}); err != nil {
		result.Error = err.Error()
		result.DurationMS = time.Since(start).Milliseconds()
		result.ExitCode = ExitConn
		return result, ExitConn
	}

	msg, err := wsjson.Read(conn)
	if err != nil {
		result.Error = err.Error()
		result.DurationMS = time.Since(start).Milliseconds()
		result.ExitCode = ExitConn
		return result, ExitConn
	}
	if msg.Type == "error" {
		result.Error = msg.Error
		result.DurationMS = time.Since(start).Milliseconds()
		result.ExitCode = ExitCommand
		return result, ExitCommand
	}
	result.Success = msg.Success
	result.DurationMS = time.Since(start).Milliseconds()
	result.ExitCode = ExitOK
	result.Output = msg.Entries
	return result, ExitOK
}

func (c *Client) Push(localPath, remotePath string, opts TransferOptions) (protocol.Result, int) {
	start := time.Now()
	result := protocol.Result{Type: "push"}
	info, err := os.Stat(localPath)
	if err != nil {
		result.Error = err.Error()
		result.DurationMS = time.Since(start).Milliseconds()
		result.ExitCode = ExitTransfer
		return result, ExitTransfer
	}
	sum, err := fileutil.SHA256File(localPath)
	if err != nil {
		result.Error = err.Error()
		result.DurationMS = time.Since(start).Milliseconds()
		result.ExitCode = ExitTransfer
		return result, ExitTransfer
	}

	conn, exitCode, err := c.connect()
	if err != nil {
		result.Error = err.Error()
		result.DurationMS = time.Since(start).Milliseconds()
		result.ExitCode = ExitConn
		return result, exitCode
	}
	defer conn.Close()

	if err := wsjson.Write(conn, protocol.Message{
		Type:      "push_start",
		Remote:    remotePath,
		Size:      info.Size(),
		SHA256:    sum,
		Force:     opts.Force,
		Skip:      opts.SkipExisting,
		ChunkSize: c.Config.ChunkSize,
	}); err != nil {
		result.Error = err.Error()
		result.DurationMS = time.Since(start).Milliseconds()
		result.ExitCode = ExitConn
		return result, ExitConn
	}

	msg, err := wsjson.Read(conn)
	if err != nil {
		result.Error = err.Error()
		result.DurationMS = time.Since(start).Milliseconds()
		result.ExitCode = ExitConn
		return result, ExitConn
	}
	if msg.Type == "error" {
		result.Error = msg.Error
		result.DurationMS = time.Since(start).Milliseconds()
		result.ExitCode = ExitTransfer
		return result, ExitTransfer
	}
	if msg.Type == "push_result" {
		result.Success = msg.Success
		result.DurationMS = time.Since(start).Milliseconds()
		result.ExitCode = ExitOK
		result.Output = map[string]any{
			"local":   localPath,
			"remote":  remotePath,
			"size":    msg.Size,
			"written": msg.Written,
			"sha256":  sum,
			"status":  fallback(msg.Error, "completed"),
		}
		return result, ExitOK
	}
	if msg.Type != "push_ready" {
		result.Error = "unexpected server response"
		result.DurationMS = time.Since(start).Milliseconds()
		result.ExitCode = ExitTransfer
		return result, ExitTransfer
	}

	file, err := os.Open(localPath)
	if err != nil {
		result.Error = err.Error()
		result.DurationMS = time.Since(start).Milliseconds()
		result.ExitCode = ExitTransfer
		return result, ExitTransfer
	}
	defer file.Close()

	if _, err := file.Seek(msg.Offset, 0); err != nil {
		result.Error = err.Error()
		result.DurationMS = time.Since(start).Milliseconds()
		result.ExitCode = ExitTransfer
		return result, ExitTransfer
	}

	buf := make([]byte, c.Config.ChunkSize)
	written := msg.Offset
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			chunk := base64.StdEncoding.EncodeToString(buf[:n])
			if err := wsjson.Write(conn, protocol.Message{Type: "push_chunk", Data: chunk}); err != nil {
				result.Error = err.Error()
				result.DurationMS = time.Since(start).Milliseconds()
				result.ExitCode = ExitTransfer
				return result, ExitTransfer
			}
			written += int64(n)
		}
		if errors.Is(readErr, os.ErrClosed) {
			result.Error = readErr.Error()
			result.DurationMS = time.Since(start).Milliseconds()
			result.ExitCode = ExitTransfer
			return result, ExitTransfer
		}
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				result.Error = readErr.Error()
				result.DurationMS = time.Since(start).Milliseconds()
				result.ExitCode = ExitTransfer
				return result, ExitTransfer
			}
			if errors.Is(readErr, io.EOF) || readErr.Error() == "EOF" {
				break
			}
			result.Error = readErr.Error()
			result.DurationMS = time.Since(start).Milliseconds()
			result.ExitCode = ExitTransfer
			return result, ExitTransfer
		}
	}

	if err := wsjson.Write(conn, protocol.Message{Type: "push_finish"}); err != nil {
		result.Error = err.Error()
		result.DurationMS = time.Since(start).Milliseconds()
		result.ExitCode = ExitTransfer
		return result, ExitTransfer
	}

	finish, err := wsjson.Read(conn)
	if err != nil {
		result.Error = err.Error()
		result.DurationMS = time.Since(start).Milliseconds()
		result.ExitCode = ExitTransfer
		return result, ExitTransfer
	}
	if finish.Type == "error" {
		result.Error = finish.Error
		result.DurationMS = time.Since(start).Milliseconds()
		result.ExitCode = ExitTransfer
		return result, ExitTransfer
	}

	result.Success = finish.Success
	result.DurationMS = time.Since(start).Milliseconds()
	result.ExitCode = ExitOK
	result.Output = map[string]any{
		"local":   localPath,
		"remote":  remotePath,
		"size":    info.Size(),
		"written": written,
		"sha256":  sum,
	}
	return result, ExitOK
}

func (c *Client) Pull(remotePath, localPath string, opts TransferOptions) (protocol.Result, int) {
	start := time.Now()
	result := protocol.Result{Type: "pull"}
	tempPath := localPath + c.Config.ResumeTempSuffix
	metaPath := fileutil.MetaPath(tempPath)

	if info, err := os.Stat(localPath); err == nil {
		if opts.SkipExisting {
			result.Success = true
			result.DurationMS = time.Since(start).Milliseconds()
			result.ExitCode = ExitOK
			result.Output = map[string]any{
				"local":  localPath,
				"remote": remotePath,
				"size":   info.Size(),
				"status": "skipped existing file",
			}
			return result, ExitOK
		}
		if !opts.Force {
			result.Error = "local file exists; use --force or --skip-existing"
			result.DurationMS = time.Since(start).Milliseconds()
			result.ExitCode = ExitTransfer
			return result, ExitTransfer
		}
		if err := os.Remove(localPath); err != nil {
			result.Error = err.Error()
			result.DurationMS = time.Since(start).Milliseconds()
			result.ExitCode = ExitTransfer
			return result, ExitTransfer
		}
	}
	if opts.Force {
		_ = fileutil.RemoveIfExists(tempPath)
		_ = fileutil.RemoveIfExists(metaPath)
	}
	retryFresh := false
	for attempt := 0; attempt < 2; attempt++ {
		offset := int64(0)
		if c.Config.ResumeEnabled && !retryFresh {
			meta, metaErr := fileutil.ReadMeta(metaPath)
			info, statErr := os.Stat(tempPath)
			switch {
			case metaErr == nil && statErr == nil && meta.TargetPath == remotePath:
				offset = info.Size()
			case statErr == nil:
				_ = fileutil.RemoveIfExists(tempPath)
				_ = fileutil.RemoveIfExists(metaPath)
			}
		}

		conn, exitCode, err := c.connect()
		if err != nil {
			result.Error = err.Error()
			result.DurationMS = time.Since(start).Milliseconds()
			result.ExitCode = ExitConn
			return result, exitCode
		}

		if err := wsjson.Write(conn, protocol.Message{
			Type:      "pull_start",
			Remote:    remotePath,
			Offset:    offset,
			ChunkSize: c.Config.ChunkSize,
		}); err != nil {
			_ = conn.Close()
			result.Error = err.Error()
			result.DurationMS = time.Since(start).Milliseconds()
			result.ExitCode = ExitConn
			return result, ExitConn
		}

		infoMsg, err := wsjson.Read(conn)
		if err != nil {
			_ = conn.Close()
			result.Error = err.Error()
			result.DurationMS = time.Since(start).Milliseconds()
			result.ExitCode = ExitConn
			return result, ExitConn
		}
		if infoMsg.Type == "error" {
			_ = conn.Close()
			result.Error = infoMsg.Error
			result.DurationMS = time.Since(start).Milliseconds()
			result.ExitCode = ExitTransfer
			return result, ExitTransfer
		}
		if infoMsg.Type != "pull_info" {
			_ = conn.Close()
			result.Error = "unexpected server response"
			result.DurationMS = time.Since(start).Milliseconds()
			result.ExitCode = ExitTransfer
			return result, ExitTransfer
		}

		if offset > 0 {
			meta, err := fileutil.ReadMeta(metaPath)
			if err != nil || meta.TargetPath != remotePath || meta.SHA256 != infoMsg.SHA256 || meta.Size != infoMsg.Size || offset > infoMsg.Size {
				_ = conn.Close()
				_ = fileutil.RemoveIfExists(tempPath)
				_ = fileutil.RemoveIfExists(metaPath)
				retryFresh = true
				continue
			}
		}

		if err := fileutil.EnsureParentDir(localPath); err != nil {
			_ = conn.Close()
			result.Error = err.Error()
			result.DurationMS = time.Since(start).Milliseconds()
			result.ExitCode = ExitTransfer
			return result, ExitTransfer
		}
		flags := os.O_CREATE | os.O_RDWR
		if offset == 0 {
			flags |= os.O_TRUNC
		}
		file, err := os.OpenFile(tempPath, flags, 0o644)
		if err != nil {
			_ = conn.Close()
			result.Error = err.Error()
			result.DurationMS = time.Since(start).Milliseconds()
			result.ExitCode = ExitTransfer
			return result, ExitTransfer
		}

		if _, err := file.Seek(offset, 0); err != nil {
			_ = file.Close()
			_ = conn.Close()
			result.Error = err.Error()
			result.DurationMS = time.Since(start).Milliseconds()
			result.ExitCode = ExitTransfer
			return result, ExitTransfer
		}
		if err := fileutil.WriteMeta(metaPath, fileutil.ResumeMeta{
			TargetPath: remotePath,
			Size:       infoMsg.Size,
			SHA256:     infoMsg.SHA256,
		}); err != nil {
			_ = file.Close()
			_ = conn.Close()
			result.Error = err.Error()
			result.DurationMS = time.Since(start).Milliseconds()
			result.ExitCode = ExitTransfer
			return result, ExitTransfer
		}

		written := offset
		for {
			msg, err := wsjson.Read(conn)
			if err != nil {
				_ = file.Close()
				_ = conn.Close()
				result.Error = err.Error()
				result.DurationMS = time.Since(start).Milliseconds()
				result.ExitCode = ExitTransfer
				return result, ExitTransfer
			}
			switch msg.Type {
			case "pull_chunk":
				data, err := base64.StdEncoding.DecodeString(msg.Data)
				if err != nil {
					_ = file.Close()
					_ = conn.Close()
					result.Error = err.Error()
					result.DurationMS = time.Since(start).Milliseconds()
					result.ExitCode = ExitTransfer
					return result, ExitTransfer
				}
				if _, err := file.Write(data); err != nil {
					_ = file.Close()
					_ = conn.Close()
					result.Error = err.Error()
					result.DurationMS = time.Since(start).Milliseconds()
					result.ExitCode = ExitTransfer
					return result, ExitTransfer
				}
				written += int64(len(data))
			case "pull_result":
				if err := file.Close(); err != nil {
					_ = conn.Close()
					result.Error = err.Error()
					result.DurationMS = time.Since(start).Milliseconds()
					result.ExitCode = ExitTransfer
					return result, ExitTransfer
				}
				_ = conn.Close()
				sum, err := fileutil.SHA256File(tempPath)
				if err != nil {
					result.Error = err.Error()
					result.DurationMS = time.Since(start).Milliseconds()
					result.ExitCode = ExitTransfer
					return result, ExitTransfer
				}
				if sum != msg.SHA256 {
					result.Error = "sha256 mismatch"
					result.DurationMS = time.Since(start).Milliseconds()
					result.ExitCode = ExitTransfer
					return result, ExitTransfer
				}
				if err := os.Rename(tempPath, localPath); err != nil {
					result.Error = err.Error()
					result.DurationMS = time.Since(start).Milliseconds()
					result.ExitCode = ExitTransfer
					return result, ExitTransfer
				}
				_ = fileutil.RemoveIfExists(metaPath)
				result.Success = true
				result.DurationMS = time.Since(start).Milliseconds()
				result.ExitCode = ExitOK
				result.Output = map[string]any{
					"local":   localPath,
					"remote":  remotePath,
					"size":    msg.Size,
					"written": written,
					"sha256":  msg.SHA256,
				}
				return result, ExitOK
			case "error":
				_ = file.Close()
				_ = conn.Close()
				result.Error = msg.Error
				result.DurationMS = time.Since(start).Milliseconds()
				result.ExitCode = ExitTransfer
				return result, ExitTransfer
			}
		}
	}

	result.Error = "pull resume retry exhausted"
	result.DurationMS = time.Since(start).Milliseconds()
	result.ExitCode = ExitTransfer
	return result, ExitTransfer
}

func LoadTasks(path string) ([]protocol.TaskLine, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var tasks []protocol.TaskLine
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var task protocol.TaskLine
		if err := json.Unmarshal([]byte(line), &task); err != nil {
			return nil, fmt.Errorf("invalid jsonl: %w", err)
		}
		tasks = append(tasks, task)
	}
	return tasks, scanner.Err()
}

func WriteTaskReport(path string, results []protocol.Result) error {
	report := protocol.TaskReport{
		Success: allSuccess(results),
		Count:   len(results),
		Results: results,
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func Cleanup(root, suffix string) (protocol.Result, int) {
	start := time.Now()
	result := protocol.Result{Type: "cleanup"}
	removed := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, suffix) || strings.HasSuffix(path, suffix+".meta.json") {
			if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
				return rmErr
			}
			removed = append(removed, path)
		}
		return nil
	})
	if err != nil {
		result.Error = err.Error()
		result.DurationMS = time.Since(start).Milliseconds()
		result.ExitCode = ExitTransfer
		return result, ExitTransfer
	}
	result.Success = true
	result.DurationMS = time.Since(start).Milliseconds()
	result.ExitCode = ExitOK
	result.Output = removed
	return result, ExitOK
}

func (c *Client) connect() (*websocket.Conn, int, error) {
	if strings.TrimSpace(c.Config.WSURL) == "" {
		return nil, ExitConn, errors.New("missing ws_url")
	}
	conn, _, err := websocket.DefaultDialer.Dial(c.Config.WSURL, nil)
	if err != nil {
		return nil, ExitConn, err
	}
	if err := wsjson.Write(conn, protocol.Message{Type: "auth", Token: c.Config.Token}); err != nil {
		_ = conn.Close()
		return nil, ExitConn, err
	}
	resp, err := wsjson.Read(conn)
	if err != nil {
		_ = conn.Close()
		return nil, ExitConn, err
	}
	if resp.Type != "auth_result" || !resp.Success {
		_ = conn.Close()
		return nil, ExitConn, fmt.Errorf("auth failed: %s", fallback(resp.Error, "unknown error"))
	}
	return conn, ExitOK, nil
}

func allSuccess(results []protocol.Result) bool {
	for _, item := range results {
		if !item.Success {
			return false
		}
	}
	return true
}

func fallback(value, fallbackValue string) string {
	if strings.TrimSpace(value) == "" {
		return fallbackValue
	}
	return value
}

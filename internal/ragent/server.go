package ragent

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/adfoke/clioverfrp/internal/config"
	"github.com/adfoke/clioverfrp/internal/fileutil"
	"github.com/adfoke/clioverfrp/internal/protocol"
	"github.com/adfoke/clioverfrp/internal/wsjson"
)

type Server struct {
	Token      string
	ChunkSize  int
	TempSuffix string
}

type pushSession struct {
	file         *os.File
	tempPath     string
	finalPath    string
	metaPath     string
	expectedSize int64
	expectedHash string
	written      int64
}

type connectionState struct {
	push *pushSession
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

func (s *Server) Run(addr string) error {
	if s.TempSuffix == "" {
		s.TempSuffix = config.DefaultTempSuffix
	}
	if s.ChunkSize <= 0 {
		s.ChunkSize = 64 * 1024
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWS)
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return server.ListenAndServe()
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	state := &connectionState{}
	defer state.closePush()

	authMsg, err := wsjson.Read(conn)
	if err != nil {
		return
	}
	if authMsg.Type != "auth" {
		_ = wsjson.Write(conn, protocol.Message{Type: "auth_result", Error: "missing auth"})
		return
	}
	if s.Token != "" && authMsg.Token != s.Token {
		_ = wsjson.Write(conn, protocol.Message{Type: "auth_result", Error: "invalid token"})
		return
	}
	if err := wsjson.Write(conn, protocol.Message{Type: "auth_result", Success: true}); err != nil {
		return
	}

	for {
		msg, err := wsjson.Read(conn)
		if err != nil {
			return
		}
		if err := s.handleMessage(conn, state, msg); err != nil {
			_ = wsjson.Write(conn, protocol.Message{Type: "error", Error: err.Error()})
		}
	}
}

func (s *Server) handleMessage(conn *websocket.Conn, state *connectionState, msg protocol.Message) error {
	switch msg.Type {
	case "info_request":
		return s.handleInfo(conn)
	case "exec_request":
		return s.handleExec(conn, msg)
	case "ls_request":
		return s.handleLS(conn, msg)
	case "push_start":
		return s.handlePushStart(conn, state, msg)
	case "push_chunk":
		return s.handlePushChunk(state, msg)
	case "push_finish":
		return s.handlePushFinish(conn, state)
	case "pull_start":
		return s.handlePull(conn, msg)
	default:
		return fmt.Errorf("unsupported message type: %s", msg.Type)
	}
}

func (s *Server) handleInfo(conn *websocket.Conn) error {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = ""
	}
	return wsjson.Write(conn, protocol.Message{
		Type:     "info_result",
		Success:  true,
		Hostname: hostname,
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
	})
}

func (s *Server) handleExec(conn *websocket.Conn, msg protocol.Message) error {
	if strings.TrimSpace(msg.Cmd) == "" {
		return errors.New("empty command")
	}

	start := time.Now()
	cmd := shellCommand(msg.Cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	var mu sync.Mutex
	stream := func(name string, r io.Reader) {
		reader := bufio.NewReader(r)
		buf := make([]byte, 4096)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				mu.Lock()
				_ = wsjson.Write(conn, protocol.Message{
					Type:   "exec_stream",
					Stream: name,
					Data:   string(buf[:n]),
				})
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		stream("stdout", stdout)
	}()
	go func() {
		defer wg.Done()
		stream("stderr", stderr)
	}()

	waitErr := cmd.Wait()
	wg.Wait()

	exitCode := 0
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return waitErr
		}
	}

	return wsjson.Write(conn, protocol.Message{
		Type:       "exec_result",
		Success:    exitCode == 0,
		DurationMS: time.Since(start).Milliseconds(),
		ExitCode:   exitCode,
	})
}

func (s *Server) handleLS(conn *websocket.Conn, msg protocol.Message) error {
	path := msg.Path
	if path == "" {
		path = "."
	}
	start := time.Now()
	entries, err := fileutil.ListDir(path)
	if err != nil {
		return err
	}
	return wsjson.Write(conn, protocol.Message{
		Type:       "ls_result",
		Success:    true,
		Entries:    entries,
		DurationMS: time.Since(start).Milliseconds(),
	})
}

func (s *Server) handlePushStart(conn *websocket.Conn, state *connectionState, msg protocol.Message) error {
	if msg.Remote == "" {
		return errors.New("missing remote path")
	}
	if msg.Size < 0 {
		return errors.New("invalid size")
	}
	state.closePush()

	finalPath := msg.Remote
	tempPath := finalPath + s.TempSuffix
	metaPath := fileutil.MetaPath(tempPath)

	if msg.Force {
		if err := fileutil.RemoveIfExists(finalPath); err != nil {
			return err
		}
		if err := fileutil.RemoveIfExists(tempPath); err != nil {
			return err
		}
		if err := fileutil.RemoveIfExists(metaPath); err != nil {
			return err
		}
	}

	if err := fileutil.EnsureParentDir(finalPath); err != nil {
		return err
	}

	if info, err := os.Stat(finalPath); err == nil {
		finalHash, hashErr := fileutil.SHA256File(finalPath)
		if hashErr != nil {
			return hashErr
		}
		if finalHash == msg.SHA256 && info.Size() == msg.Size {
			return wsjson.Write(conn, protocol.Message{
				Type:       "push_result",
				Success:    true,
				Size:       msg.Size,
				Written:    msg.Size,
				DurationMS: 0,
			})
		}
		if msg.Skip {
			return wsjson.Write(conn, protocol.Message{
				Type:    "push_result",
				Success: true,
				Size:    info.Size(),
				Written: 0,
				Error:   "skipped existing file",
			})
		}
		return errors.New("remote file exists; use --force or --skip-existing")
	}

	offset := int64(0)
	if meta, err := fileutil.ReadMeta(metaPath); err == nil {
		if meta.TargetPath == finalPath && meta.Size == msg.Size && meta.SHA256 == msg.SHA256 {
			if info, err := os.Stat(tempPath); err == nil {
				offset = info.Size()
			}
		} else {
			_ = fileutil.RemoveIfExists(tempPath)
			_ = fileutil.RemoveIfExists(metaPath)
		}
	}

	if offset > msg.Size {
		offset = 0
		_ = fileutil.RemoveIfExists(tempPath)
		_ = fileutil.RemoveIfExists(metaPath)
	}

	flags := os.O_CREATE | os.O_RDWR
	if offset == 0 {
		flags |= os.O_TRUNC
	}
	file, err := os.OpenFile(tempPath, flags, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		_ = file.Close()
		return err
	}

	if err := fileutil.WriteMeta(metaPath, fileutil.ResumeMeta{
		TargetPath: finalPath,
		Size:       msg.Size,
		SHA256:     msg.SHA256,
	}); err != nil {
		_ = file.Close()
		return err
	}

	state.push = &pushSession{
		file:         file,
		tempPath:     tempPath,
		finalPath:    finalPath,
		metaPath:     metaPath,
		expectedSize: msg.Size,
		expectedHash: msg.SHA256,
		written:      offset,
	}
	return wsjson.Write(conn, protocol.Message{
		Type:    "push_ready",
		Success: true,
		Offset:  offset,
	})
}

func (s *Server) handlePushChunk(state *connectionState, msg protocol.Message) error {
	if state.push == nil {
		return errors.New("push session not started")
	}
	data, err := base64.StdEncoding.DecodeString(msg.Data)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	if _, err := state.push.file.Write(data); err != nil {
		return err
	}
	state.push.written += int64(len(data))
	return nil
}

func (s *Server) handlePushFinish(conn *websocket.Conn, state *connectionState) error {
	if state.push == nil {
		return errors.New("push session not started")
	}
	session := state.push
	state.push = nil

	if err := session.file.Close(); err != nil {
		return err
	}
	if session.written != session.expectedSize {
		return fmt.Errorf("transfer incomplete: expected %d bytes, got %d", session.expectedSize, session.written)
	}
	sum, err := fileutil.SHA256File(session.tempPath)
	if err != nil {
		return err
	}
	if sum != session.expectedHash {
		return errors.New("sha256 mismatch")
	}
	if err := os.Rename(session.tempPath, session.finalPath); err != nil {
		return err
	}
	if err := fileutil.RemoveIfExists(session.metaPath); err != nil {
		return err
	}
	return wsjson.Write(conn, protocol.Message{
		Type:    "push_result",
		Success: true,
		Size:    session.expectedSize,
		Written: session.expectedSize,
		SHA256:  session.expectedHash,
	})
}

func (s *Server) handlePull(conn *websocket.Conn, msg protocol.Message) error {
	if msg.Remote == "" {
		return errors.New("missing remote path")
	}
	file, err := os.Open(msg.Remote)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}
	sum, err := fileutil.SHA256File(msg.Remote)
	if err != nil {
		return err
	}

	offset := msg.Offset
	if offset < 0 || offset > info.Size() {
		offset = 0
	}

	if err := wsjson.Write(conn, protocol.Message{
		Type:    "pull_info",
		Success: true,
		Size:    info.Size(),
		SHA256:  sum,
		Offset:  offset,
	}); err != nil {
		return err
	}

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	chunkSize := msg.ChunkSize
	if chunkSize <= 0 {
		chunkSize = s.ChunkSize
	}

	buf := make([]byte, chunkSize)
	sent := offset
	for {
		n, err := file.Read(buf)
		if n > 0 {
			if writeErr := wsjson.Write(conn, protocol.Message{
				Type:   "pull_chunk",
				Offset: sent,
				Data:   base64.StdEncoding.EncodeToString(buf[:n]),
			}); writeErr != nil {
				return writeErr
			}
			sent += int64(n)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}

	return wsjson.Write(conn, protocol.Message{
		Type:    "pull_result",
		Success: true,
		Size:    info.Size(),
		Written: sent,
		SHA256:  sum,
		EOF:     true,
	})
}

func (s *connectionState) closePush() {
	if s.push == nil {
		return
	}
	_ = s.push.file.Close()
	s.push = nil
}

func shellCommand(command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(context.Background(), "cmd", "/C", command)
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return exec.CommandContext(context.Background(), shell, "-lc", command)
}

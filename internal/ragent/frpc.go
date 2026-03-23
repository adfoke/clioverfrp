package ragent

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type FRPCOptions struct {
	Enabled    bool
	BinaryPath string
	ConfigPath string

	ServerAddr string
	ServerPort int
	AuthToken  string
	RemotePort int
	ProxyName  string
}

func (o FRPCOptions) Validate() error {
	if !o.Enabled {
		return nil
	}
	if strings.TrimSpace(o.BinaryPath) == "" {
		return fmt.Errorf("missing frpc binary path")
	}
	if strings.TrimSpace(o.ConfigPath) != "" {
		return nil
	}
	if strings.TrimSpace(o.ServerAddr) == "" {
		return fmt.Errorf("missing frpc server addr")
	}
	if o.ServerPort <= 0 {
		return fmt.Errorf("missing frpc server port")
	}
	if strings.TrimSpace(o.AuthToken) == "" {
		return fmt.Errorf("missing frpc auth token")
	}
	if o.RemotePort <= 0 {
		return fmt.Errorf("missing frpc remote port")
	}
	return nil
}

func StartManagedFRPC(opts FRPCOptions, listenAddr string) (func(), error) {
	if !opts.Enabled {
		return func() {}, nil
	}
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	configPath := opts.ConfigPath
	cleanupConfig := func() {}
	if configPath == "" {
		localIP, localPort, err := splitListenAddr(listenAddr)
		if err != nil {
			return nil, err
		}
		content := buildFRPCConfig(opts, localIP, localPort)
		file, err := os.CreateTemp("", "clioverfrp-frpc-*.toml")
		if err != nil {
			return nil, err
		}
		if _, err := file.WriteString(content); err != nil {
			_ = file.Close()
			_ = os.Remove(file.Name())
			return nil, err
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(file.Name())
			return nil, err
		}
		configPath = file.Name()
		cleanupConfig = func() {
			_ = os.Remove(configPath)
		}
	}

	cmd := exec.Command(opts.BinaryPath, "-c", configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cleanupConfig()
		return nil, err
	}

	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	stop := func() {
		defer cleanupConfig()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
	return stop, nil
}

func buildFRPCConfig(opts FRPCOptions, localIP string, localPort int) string {
	proxyName := opts.ProxyName
	if strings.TrimSpace(proxyName) == "" {
		proxyName = "clioverfrp"
	}
	return fmt.Sprintf(`serverAddr = %q
serverPort = %d

auth.method = "token"
auth.token = %q

[[proxies]]
name = %q
type = "tcp"
localIP = %q
localPort = %d
remotePort = %d
`, opts.ServerAddr, opts.ServerPort, opts.AuthToken, proxyName, localIP, localPort, opts.RemotePort)
}

func splitListenAddr(addr string) (string, int, error) {
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid listen addr %q: %w", addr, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return "", 0, fmt.Errorf("invalid listen port %q: %w", portText, err)
	}
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}
	return host, port, nil
}

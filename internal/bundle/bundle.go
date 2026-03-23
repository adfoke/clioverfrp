package bundle

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/adfoke/clioverfrp/internal/config"
)

func Build(cfg config.Config) error {
	if cfg.BundleFRPCBin == "" {
		cfg.BundleFRPCBin = cfg.FRPCBin
	}
	if cfg.BundleFRPCBin == "" {
		return fmt.Errorf("missing frpc binary; set bundle.frpc_bin or frpc.bin in config.yaml")
	}
	if cfg.BundleOutputDir == "" {
		cfg.BundleOutputDir = "dist/ragent-bundle"
	}
	if cfg.BundleRagentBin == "" {
		cfg.BundleRagentBin = "bin/ragent"
	}
	ragentBin, err := resolveBinaryPath(cfg.BundleRagentBin, "ragent")
	if err != nil {
		return err
	}
	frpcBin, err := resolveBinaryPath(cfg.BundleFRPCBin, "frpc")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(cfg.BundleOutputDir, 0o755); err != nil {
		return err
	}
	if err := copyFile(ragentBin, filepath.Join(cfg.BundleOutputDir, "ragent"), 0o755); err != nil {
		return err
	}
	if err := copyFile(frpcBin, filepath.Join(cfg.BundleOutputDir, "frpc"), 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(cfg.BundleOutputDir, "frpc.toml"), []byte(renderFRPCTOML(cfg)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(cfg.BundleOutputDir, "start.sh"), []byte(renderStartScript(cfg)), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(cfg.BundleOutputDir, "README.txt"), []byte(renderReadme(cfg)), 0o644); err != nil {
		return err
	}
	return nil
}

func resolveBinaryPath(path, name string) (string, error) {
	if strings.Contains(path, "/") || strings.Contains(path, string(filepath.Separator)) || strings.HasPrefix(path, ".") {
		info, err := os.Stat(path)
		if err != nil {
			return "", fmt.Errorf("missing %s binary: %s", name, path)
		}
		if info.Mode()&0o111 == 0 {
			return "", fmt.Errorf("%s not executable: %s", name, path)
		}
		return path, nil
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return "", fmt.Errorf("missing %s binary: %s", name, path)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("missing %s binary: %s", name, resolved)
	}
	if info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("%s not executable: %s", name, resolved)
	}
	return resolved, nil
}

func mustExecutable(path, name string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("missing %s binary: %s", name, path)
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("%s not executable: %s", name, path)
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, mode)
}

func renderFRPCTOML(cfg config.Config) string {
	return fmt.Sprintf(`serverAddr = %q
serverPort = %d

auth.method = "token"
auth.token = %q

[[proxies]]
name = %q
type = "tcp"
localIP = "127.0.0.1"
localPort = 9000
remotePort = %d
`, cfg.FRPCServerAddr, cfg.FRPCServerPort, cfg.FRPCAuthToken, cfg.FRPCProxyName, cfg.FRPCRemotePort)
}

func renderStartScript(cfg config.Config) string {
	return fmt.Sprintf(`#!/bin/sh
set -eu
DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
export CLIOVERFRP_TOKEN=%q
exec "$DIR/ragent" --listen %q --frpc-config "$DIR/frpc.toml" --frpc-bin "$DIR/frpc"
`, cfg.Token, cfg.ListenAddr)
}

func renderReadme(cfg config.Config) string {
	return fmt.Sprintf(`1. 把这个目录发到内网机器
2. 确认 frpc.toml 里的 serverAddr / token / remotePort 正确
3. 执行: ./start.sh

当前默认监听:
%s
`, cfg.ListenAddr)
}

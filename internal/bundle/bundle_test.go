package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adfoke/clioverfrp/internal/config"
)

func TestBuild(t *testing.T) {
	dir := t.TempDir()
	ragent := filepath.Join(dir, "ragent-src")
	frpc := filepath.Join(dir, "frpc-src")
	for _, path := range []string{ragent, frpc} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	cfg := config.Default()
	cfg.Token = "agent-token"
	cfg.ListenAddr = "127.0.0.1:9000"
	cfg.FRPCServerAddr = "1.2.3.4"
	cfg.FRPCAuthToken = "frp-token"
	cfg.FRPCRemotePort = 60001
	cfg.BundleOutputDir = filepath.Join(dir, "bundle")
	cfg.BundleRagentBin = ragent
	cfg.BundleFRPCBin = frpc

	if err := Build(cfg); err != nil {
		t.Fatal(err)
	}

	checks := []string{
		filepath.Join(cfg.BundleOutputDir, "ragent"),
		filepath.Join(cfg.BundleOutputDir, "frpc"),
		filepath.Join(cfg.BundleOutputDir, "frpc.toml"),
		filepath.Join(cfg.BundleOutputDir, "start.sh"),
		filepath.Join(cfg.BundleOutputDir, "README.txt"),
	}
	for _, path := range checks {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing bundle file %s: %v", path, err)
		}
	}
}

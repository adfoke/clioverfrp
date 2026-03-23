package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUsesConfigAndEnvOverrides(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := `agent:
  token: "file-token"
agents:
  default: "dev"
  dev:
    ws_url: "ws://dev-from-file/ws"
    token: "dev-file-token"
  prod:
    ws_url: "ws://prod-from-file/ws"
    token: "prod-file-token"
lagent:
  ws_url: "ws://from-file/ws"
  json_output: false
  quiet_mode: false
  resume_enabled: false
  resume_temp_suffix: ".resume"
  max_retry: 9
  chunk_size: 2048
ragent:
  listen: "127.0.0.1:9100"
frpc:
  enable: true
  server_addr: "1.2.3.4"
  server_port: 7001
  auth_token: "frp-file-token"
  remote_port: 60002
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CLIOVERFRP_CONFIG", configPath)
	t.Setenv("CLIOVERFRP_WS_URL", "ws://from-env/ws")
	t.Setenv("CLIOVERFRP_TOKEN", "env-token")
	t.Setenv("CLIOVERFRP_JSON", "true")
	t.Setenv("CLIOVERFRP_QUIET", "true")
	t.Setenv("CLIOVERFRP_RESUME", "true")
	t.Setenv("CLIOVERFRP_RESUME_TEMP_SUFFIX", ".envtmp")
	t.Setenv("CLIOVERFRP_MAX_RETRY", "12")
	t.Setenv("CLIOVERFRP_CHUNK_SIZE", "4096")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.WSURL != "ws://from-env/ws" {
		t.Fatalf("unexpected ws_url: %q", cfg.WSURL)
	}
	if cfg.Token != "env-token" {
		t.Fatalf("unexpected token: %q", cfg.Token)
	}
	if !cfg.JSONOutput || !cfg.QuietMode || !cfg.ResumeEnabled {
		t.Fatalf("env bool overrides not applied: %+v", cfg)
	}
	if cfg.ResumeTempSuffix != ".envtmp" {
		t.Fatalf("unexpected temp suffix: %q", cfg.ResumeTempSuffix)
	}
	if cfg.MaxRetry != 12 {
		t.Fatalf("unexpected max_retry: %d", cfg.MaxRetry)
	}
	if cfg.ChunkSize != 4096 {
		t.Fatalf("unexpected chunk_size: %d", cfg.ChunkSize)
	}
	if cfg.ListenAddr != "127.0.0.1:9100" {
		t.Fatalf("unexpected listen addr: %q", cfg.ListenAddr)
	}
	if !cfg.FRPCEnable || cfg.FRPCServerAddr != "1.2.3.4" || cfg.FRPCServerPort != 7001 || cfg.FRPCRemotePort != 60002 {
		t.Fatalf("unexpected frpc config: %+v", cfg)
	}
	if cfg.DefaultAgent != "dev" {
		t.Fatalf("unexpected default agent: %q", cfg.DefaultAgent)
	}
	if cfg.Agents["prod"].WSURL != "ws://prod-from-file/ws" || cfg.Agents["prod"].Token != "prod-file-token" {
		t.Fatalf("unexpected agents map: %+v", cfg.Agents)
	}
}

func TestLoadFallsBackToDefaultsOnInvalidValues(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := `lagent:
  resume_temp_suffix: ""
  max_retry: -1
  chunk_size: 0
ragent:
  listen: ""
frpc:
  bin: ""
  server_port: 0
  proxy_name: ""
bundle:
  output_dir: ""
  ragent_bin: ""
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CLIOVERFRP_CONFIG", configPath)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.ResumeTempSuffix != DefaultTempSuffix {
		t.Fatalf("unexpected default temp suffix: %q", cfg.ResumeTempSuffix)
	}
	if cfg.MaxRetry != 5 {
		t.Fatalf("unexpected default max_retry: %d", cfg.MaxRetry)
	}
	if cfg.ChunkSize != 64*1024 {
		t.Fatalf("unexpected default chunk_size: %d", cfg.ChunkSize)
	}
	if cfg.ListenAddr != "127.0.0.1:9000" || cfg.FRPCBin != "frpc" || cfg.FRPCServerPort != 7000 || cfg.FRPCProxyName != "clioverfrp" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.BundleOutputDir != "dist/ragent-bundle" || cfg.BundleRagentBin != "bin/ragent" {
		t.Fatalf("unexpected bundle defaults: %+v", cfg)
	}
}

func TestResolveAgent(t *testing.T) {
	cfg := Default()
	cfg.WSURL = "ws://single/ws"
	cfg.Token = "single-token"
	cfg.Agents["dev"] = AgentConfig{WSURL: "ws://dev/ws", Token: "dev-token"}
	cfg.Agents["prod"] = AgentConfig{WSURL: "ws://prod/ws", Token: "prod-token"}
	cfg.DefaultAgent = "prod"

	resolved, err := cfg.ResolveAgent("")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.WSURL != "ws://prod/ws" || resolved.Token != "prod-token" {
		t.Fatalf("unexpected resolved default agent: %+v", resolved)
	}

	resolved, err = cfg.ResolveAgent("dev")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.WSURL != "ws://dev/ws" || resolved.Token != "dev-token" {
		t.Fatalf("unexpected resolved named agent: %+v", resolved)
	}

	if _, err := cfg.ResolveAgent("missing"); err == nil {
		t.Fatal("expected unknown agent error")
	}
}

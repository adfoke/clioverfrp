package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const DefaultTempSuffix = ".clioverfrp.tmp"

type Config struct {
	WSURL            string
	Token            string
	DefaultAgent     string
	Agents           map[string]AgentConfig
	JSONOutput       bool
	QuietMode        bool
	ResumeEnabled    bool
	ResumeTempSuffix string
	MaxRetry         int
	ChunkSize        int
	ListenAddr       string
	FRPCEnable       bool
	FRPCBin          string
	FRPCConfig       string
	FRPCServerAddr   string
	FRPCServerPort   int
	FRPCAuthToken    string
	FRPCRemotePort   int
	FRPCProxyName    string
	BundleOutputDir  string
	BundleRagentBin  string
	BundleFRPCBin    string
}

type AgentConfig struct {
	WSURL string
	Token string
}

type AgentInfo struct {
	Name       string `json:"name"`
	WSURL      string `json:"ws_url"`
	IsDefault  bool   `json:"is_default"`
	HasToken   bool   `json:"has_token"`
	IsImplicit bool   `json:"is_implicit"`
}

func Default() Config {
	return Config{
		Agents:           map[string]AgentConfig{},
		JSONOutput:       false,
		QuietMode:        false,
		ResumeEnabled:    true,
		ResumeTempSuffix: DefaultTempSuffix,
		MaxRetry:         5,
		ChunkSize:        64 * 1024,
		ListenAddr:       "127.0.0.1:9000",
		FRPCEnable:       false,
		FRPCBin:          "frpc",
		FRPCServerPort:   7000,
		FRPCProxyName:    "clioverfrp",
		BundleOutputDir:  "dist/ragent-bundle",
		BundleRagentBin:  "bin/ragent",
	}
}

func Load() (Config, error) {
	cfg := Default()
	if err := loadConfigFile(&cfg); err != nil {
		return cfg, err
	}
	applyEnv(&cfg)
	if cfg.ResumeTempSuffix == "" {
		cfg.ResumeTempSuffix = DefaultTempSuffix
	}
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 64 * 1024
	}
	if cfg.MaxRetry <= 0 {
		cfg.MaxRetry = 5
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:9000"
	}
	if cfg.FRPCBin == "" {
		cfg.FRPCBin = "frpc"
	}
	if cfg.FRPCServerPort <= 0 {
		cfg.FRPCServerPort = 7000
	}
	if cfg.FRPCProxyName == "" {
		cfg.FRPCProxyName = "clioverfrp"
	}
	if cfg.BundleOutputDir == "" {
		cfg.BundleOutputDir = "dist/ragent-bundle"
	}
	if cfg.BundleRagentBin == "" {
		cfg.BundleRagentBin = "bin/ragent"
	}
	if cfg.Agents == nil {
		cfg.Agents = map[string]AgentConfig{}
	}
	return cfg, nil
}

func loadConfigFile(cfg *Config) error {
	path, err := resolveConfigPath()
	if err != nil || path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	section := ""
	child := ""
	for scanner.Scan() {
		raw := scanner.Text()
		if idx := strings.Index(raw, "#"); idx >= 0 {
			raw = raw[:idx]
		}
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])

		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		level := indent / 2
		if level == 0 && val == "" {
			section = key
			child = ""
			continue
		}
		if level == 1 && val == "" {
			child = key
			continue
		}

		val = trimYAMLValue(val)
		if level >= 2 && section != "" && child != "" {
			applyNestedConfigValue(cfg, section, child, key, val)
			continue
		}
		if level == 1 && section != "" {
			applyConfigValue(cfg, section, key, val)
			continue
		}
		applyConfigValue(cfg, "", key, val)
	}
	return scanner.Err()
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("CLIOVERFRP_WS_URL"); v != "" {
		cfg.WSURL = v
	}
	if v := os.Getenv("CLIOVERFRP_TOKEN"); v != "" {
		cfg.Token = v
	}
	if v := os.Getenv("CLIOVERFRP_JSON"); v != "" {
		cfg.JSONOutput = parseBool(v, cfg.JSONOutput)
	}
	if v := os.Getenv("CLIOVERFRP_QUIET"); v != "" {
		cfg.QuietMode = parseBool(v, cfg.QuietMode)
	}
	if v := os.Getenv("CLIOVERFRP_RESUME"); v != "" {
		cfg.ResumeEnabled = parseBool(v, cfg.ResumeEnabled)
	}
	if v := os.Getenv("CLIOVERFRP_RESUME_TEMP_SUFFIX"); v != "" {
		cfg.ResumeTempSuffix = v
	}
	if v := os.Getenv("CLIOVERFRP_MAX_RETRY"); v != "" {
		cfg.MaxRetry = parseInt(v, cfg.MaxRetry)
	}
	if v := os.Getenv("CLIOVERFRP_CHUNK_SIZE"); v != "" {
		cfg.ChunkSize = parseInt(v, cfg.ChunkSize)
	}
	if v := os.Getenv("CLIOVERFRP_LISTEN"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("CLIOVERFRP_FRPC_ENABLE"); v != "" {
		cfg.FRPCEnable = parseBool(v, cfg.FRPCEnable)
	}
	if v := os.Getenv("CLIOVERFRP_FRPC_BIN"); v != "" {
		cfg.FRPCBin = v
	}
	if v := os.Getenv("CLIOVERFRP_FRPC_CONFIG"); v != "" {
		cfg.FRPCConfig = v
	}
	if v := os.Getenv("CLIOVERFRP_FRPC_SERVER_ADDR"); v != "" {
		cfg.FRPCServerAddr = v
	}
	if v := os.Getenv("CLIOVERFRP_FRPC_SERVER_PORT"); v != "" {
		cfg.FRPCServerPort = parseInt(v, cfg.FRPCServerPort)
	}
	if v := os.Getenv("CLIOVERFRP_FRPC_AUTH_TOKEN"); v != "" {
		cfg.FRPCAuthToken = v
	}
	if v := os.Getenv("CLIOVERFRP_FRPC_REMOTE_PORT"); v != "" {
		cfg.FRPCRemotePort = parseInt(v, cfg.FRPCRemotePort)
	}
	if v := os.Getenv("CLIOVERFRP_FRPC_PROXY_NAME"); v != "" {
		cfg.FRPCProxyName = v
	}
	if v := os.Getenv("CLIOVERFRP_BUNDLE_OUTPUT_DIR"); v != "" {
		cfg.BundleOutputDir = v
	}
	if v := os.Getenv("CLIOVERFRP_BUNDLE_RAGENT_BIN"); v != "" {
		cfg.BundleRagentBin = v
	}
	if v := os.Getenv("CLIOVERFRP_BUNDLE_FRPC_BIN"); v != "" {
		cfg.BundleFRPCBin = v
	}
}

func resolveConfigPath() (string, error) {
	if v := os.Getenv("CLIOVERFRP_CONFIG"); v != "" {
		return v, nil
	}
	candidates := []string{"config.yaml", "config.yml"}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	candidates = []string{
		filepath.Join(home, ".config", "clioverfrp", "config.yaml"),
		filepath.Join(home, ".config", "clioverfrp", "config.yml"),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", nil
}

func applyConfigValue(cfg *Config, section, key, value string) {
	switch section {
	case "agent":
		if key == "token" {
			cfg.Token = value
		}
	case "agents":
		if key == "default" {
			cfg.DefaultAgent = value
		}
	case "lagent":
		switch key {
		case "ws_url":
			cfg.WSURL = value
		case "json_output":
			cfg.JSONOutput = parseBool(value, cfg.JSONOutput)
		case "quiet_mode":
			cfg.QuietMode = parseBool(value, cfg.QuietMode)
		case "resume_enabled":
			cfg.ResumeEnabled = parseBool(value, cfg.ResumeEnabled)
		case "resume_temp_suffix":
			cfg.ResumeTempSuffix = value
		case "max_retry":
			cfg.MaxRetry = parseInt(value, cfg.MaxRetry)
		case "chunk_size":
			cfg.ChunkSize = parseInt(value, cfg.ChunkSize)
		}
	case "ragent":
		switch key {
		case "listen":
			cfg.ListenAddr = value
		}
	case "frpc":
		switch key {
		case "enable":
			cfg.FRPCEnable = parseBool(value, cfg.FRPCEnable)
		case "bin":
			cfg.FRPCBin = value
		case "config":
			cfg.FRPCConfig = value
		case "server_addr":
			cfg.FRPCServerAddr = value
		case "server_port":
			cfg.FRPCServerPort = parseInt(value, cfg.FRPCServerPort)
		case "auth_token":
			cfg.FRPCAuthToken = value
		case "remote_port":
			cfg.FRPCRemotePort = parseInt(value, cfg.FRPCRemotePort)
		case "proxy_name":
			cfg.FRPCProxyName = value
		}
	case "bundle":
		switch key {
		case "output_dir":
			cfg.BundleOutputDir = value
		case "ragent_bin":
			cfg.BundleRagentBin = value
		case "frpc_bin":
			cfg.BundleFRPCBin = value
		}
	case "":
		switch key {
		case "token":
			cfg.Token = value
		case "ws_url":
			cfg.WSURL = value
		case "listen":
			cfg.ListenAddr = value
		}
	}
}

func applyNestedConfigValue(cfg *Config, section, child, key, value string) {
	switch section {
	case "agents":
		agent := cfg.Agents[child]
		switch key {
		case "ws_url":
			agent.WSURL = value
		case "token":
			agent.Token = value
		}
		cfg.Agents[child] = agent
	}
}

func trimYAMLValue(v string) string {
	v = strings.TrimSpace(v)
	v = strings.Trim(v, "\"")
	v = strings.Trim(v, "'")
	return v
}

func WriteExample(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return err
	}
	content := fmt.Sprintf(`agent:
  token: "replace-with-your-agent-token"

agents:
  default: "dev"
  dev:
    ws_url: "ws://YOUR_FRPS_IP:60001/ws"
    token: "replace-with-your-agent-token"
  prod:
    ws_url: "ws://YOUR_FRPS_IP:60002/ws"
    token: "replace-with-your-prod-agent-token"

lagent:
  ws_url: "ws://YOUR_FRPS_IP:60001/ws"
  json_output: false
  quiet_mode: false
  resume_enabled: true
  resume_temp_suffix: %q
  max_retry: 5
  chunk_size: 65536

ragent:
  listen: "127.0.0.1:9000"

frpc:
  enable: true
  bin: "./frpc"
  config: ""
  server_addr: "YOUR_FRPS_IP"
  server_port: 7000
  auth_token: "replace-with-your-frp-token"
  remote_port: 60001
  proxy_name: "clioverfrp"

bundle:
  output_dir: "dist/ragent-bundle"
  ragent_bin: "bin/ragent"
  frpc_bin: "./frpc"
`, DefaultTempSuffix)
	return os.WriteFile(path, []byte(content), 0o644)
}

func (cfg Config) ResolveAgent(name string) (Config, error) {
	out := cfg
	if cfg.Agents == nil {
		cfg.Agents = map[string]AgentConfig{}
	}

	if name == "" {
		if cfg.DefaultAgent != "" {
			name = cfg.DefaultAgent
		} else if len(cfg.Agents) == 1 {
			for key := range cfg.Agents {
				name = key
			}
		}
	}

	if name == "" {
		if out.WSURL != "" || out.Token != "" {
			return out, nil
		}
		if len(cfg.Agents) > 1 {
			return out, fmt.Errorf("multiple agents configured; use --agent or set agents.default")
		}
		return out, nil
	}

	agent, ok := cfg.Agents[name]
	if !ok {
		return out, fmt.Errorf("unknown agent: %s", name)
	}
	out.DefaultAgent = name
	if agent.WSURL != "" {
		out.WSURL = agent.WSURL
	}
	if agent.Token != "" {
		out.Token = agent.Token
	}
	return out, nil
}

func (cfg Config) ListAgents() []AgentInfo {
	items := make([]AgentInfo, 0)
	if len(cfg.Agents) == 0 {
		if cfg.WSURL != "" || cfg.Token != "" {
			items = append(items, AgentInfo{
				Name:       "default",
				WSURL:      cfg.WSURL,
				IsDefault:  true,
				HasToken:   cfg.Token != "",
				IsImplicit: true,
			})
		}
		return items
	}

	names := make([]string, 0, len(cfg.Agents))
	for name := range cfg.Agents {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		agent := cfg.Agents[name]
		items = append(items, AgentInfo{
			Name:      name,
			WSURL:     agent.WSURL,
			IsDefault: name == cfg.DefaultAgent,
			HasToken:  agent.Token != "",
		})
	}
	return items
}

func parseBool(v string, fallback bool) bool {
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return fallback
	}
	return b
}

func parseInt(v string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return fallback
	}
	return n
}

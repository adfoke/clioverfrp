package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/adfoke/clioverfrp/internal/config"
	"github.com/adfoke/clioverfrp/internal/ragent"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	addr := flag.String("listen", cfg.ListenAddr, "listen address")
	token := flag.String("token", cfg.Token, "shared token")
	frpcEnable := flag.Bool("frpc-enable", cfg.FRPCEnable, "start frpc as child process")
	frpcBin := flag.String("frpc-bin", cfg.FRPCBin, "frpc binary path")
	frpcConfig := flag.String("frpc-config", cfg.FRPCConfig, "existing frpc config path")
	frpcServerAddr := flag.String("frpc-server-addr", cfg.FRPCServerAddr, "frps server addr")
	frpcServerPort := flag.Int("frpc-server-port", cfg.FRPCServerPort, "frps server port")
	frpcAuthToken := flag.String("frpc-auth-token", cfg.FRPCAuthToken, "frps auth token")
	frpcRemotePort := flag.Int("frpc-remote-port", cfg.FRPCRemotePort, "frp remote port")
	frpcProxyName := flag.String("frpc-proxy-name", cfg.FRPCProxyName, "frp proxy name")
	flag.Parse()

	stopFRPC, err := ragent.StartManagedFRPC(ragent.FRPCOptions{
		Enabled:    *frpcEnable || *frpcConfig != "" || *frpcServerAddr != "" || *frpcRemotePort > 0,
		BinaryPath: *frpcBin,
		ConfigPath: *frpcConfig,
		ServerAddr: *frpcServerAddr,
		ServerPort: *frpcServerPort,
		AuthToken:  *frpcAuthToken,
		RemotePort: *frpcRemotePort,
		ProxyName:  *frpcProxyName,
	}, *addr)
	if err != nil {
		return err
	}
	defer stopFRPC()

	server := &ragent.Server{
		Token:      *token,
		ChunkSize:  config.Default().ChunkSize,
		TempSuffix: config.DefaultTempSuffix,
	}
	return server.Run(*addr)
}

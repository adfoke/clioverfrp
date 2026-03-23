package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/adfoke/clioverfrp/internal/bundle"
	"github.com/adfoke/clioverfrp/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	outputDir := flag.String("output-dir", cfg.BundleOutputDir, "bundle output dir")
	ragentBin := flag.String("ragent-bin", cfg.BundleRagentBin, "ragent binary path")
	frpcBin := flag.String("frpc-bin", firstNonEmpty(cfg.BundleFRPCBin, cfg.FRPCBin), "frpc binary path")
	writeExample := flag.Bool("write-example-config", false, "write example config.yaml")
	flag.Parse()

	if *writeExample {
		if err := config.WriteExample("config.yaml"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	cfg.BundleOutputDir = *outputDir
	cfg.BundleRagentBin = *ragentBin
	cfg.BundleFRPCBin = *frpcBin

	if err := bundle.Build(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(cfg.BundleOutputDir)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

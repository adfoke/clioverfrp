package ragent

import (
	"strings"
	"testing"
)

func TestBuildFRPCConfig(t *testing.T) {
	content := buildFRPCConfig(FRPCOptions{
		ServerAddr: "1.2.3.4",
		ServerPort: 7000,
		AuthToken:  "frp-token",
		RemotePort: 60001,
		ProxyName:  "clioverfrp",
	}, "127.0.0.1", 9000)

	checks := []string{
		`serverAddr = "1.2.3.4"`,
		`serverPort = 7000`,
		`auth.token = "frp-token"`,
		`name = "clioverfrp"`,
		`localIP = "127.0.0.1"`,
		`localPort = 9000`,
		`remotePort = 60001`,
	}
	for _, item := range checks {
		if !strings.Contains(content, item) {
			t.Fatalf("missing %q in config:\n%s", item, content)
		}
	}
}

func TestSplitListenAddr(t *testing.T) {
	ip, port, err := splitListenAddr("0.0.0.0:9000")
	if err != nil {
		t.Fatal(err)
	}
	if ip != "127.0.0.1" || port != 9000 {
		t.Fatalf("unexpected split result: %s %d", ip, port)
	}
}

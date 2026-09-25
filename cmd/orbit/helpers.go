package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/mrsirg97-rgb/orbit/client"
)

func walletSuffix(pubkey string) string {
	s := pubkey
	if len(s) > 4 {
		s = s[len(s)-4:]
	}
	return strings.ToUpper(s)
}

func rigModuleVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, m := range bi.Deps {
			if m.Path == "github.com/mrsirg97-rgb/rig" && m.Version != "" {
				return m.Version
			}
		}
	}
	return "unknown"
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "orbit: "+format+"\n", a...)
	os.Exit(1)
}

func mustOrbitHome() string {
	home, err := client.Home(os.Getenv)
	if err != nil {
		die("%v", err)
	}
	return home
}

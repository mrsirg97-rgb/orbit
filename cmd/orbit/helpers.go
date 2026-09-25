package main

import (
	"fmt"
	"os"
	"strings"
)

func walletSuffix(pubkey string) string {
	s := pubkey
	if len(s) > 4 {
		s = s[len(s)-4:]
	}
	return strings.ToUpper(s)
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "orbit: "+format+"\n", a...)
	os.Exit(1)
}

func mustRigHome() string {
	home, err := rigHome()
	if err != nil {
		die("%v", err)
	}
	return home
}

package main

import (
	"fmt"
	"os"

	"github.com/asolyakov/obftun/internal/transport"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: test-token <secret>")
		os.Exit(2)
	}

	clientID, err := transport.GenerateClientID()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	gcm, err := transport.NewGCM(transport.DeriveKey(os.Args[1]))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	token, err := transport.EncryptToken(gcm, clientID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(token)
}

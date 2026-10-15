// whisk-sign is the release signing tool `make cli-dist` and `make release-key` run: it makes
// an Ed25519 key pair and signs SHA256SUMS with it (CLI.md §2). Not shipped to users.
package main

import (
	"fmt"
	"os"

	"github.com/whisk-run/cli/internal/release"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "whisk-sign:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	switch {
	case len(args) == 2 && args[0] == "keygen":
		return keygen(args[1])
	case len(args) == 2 && args[0] == "pubkey":
		return pubkey(args[1])
	case len(args) == 4 && args[0] == "verify":
		return verify(args[1], args[2], args[3])
	case len(args) == 3 && args[0] == "sign":
		return sign(args[1], args[2])
	}
	return fmt.Errorf("usage: whisk-sign keygen <key file> | whisk-sign pubkey <key file> | whisk-sign sign <key file> <file to sign> | whisk-sign verify <public key> <file> <signature file>")
}

// keygen writes the private key to path (mode 0600, refusing to overwrite) and prints the
// public key to paste into cli/whisk/release_pubkey.go.
func keygen(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s exists; remove it first if you mean to rotate the key", path)
	}
	kp, err := release.GenerateKey()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(kp.Private+"\n"), 0o600); err != nil {
		return err
	}
	fmt.Printf("private key written to %s (keep it out of the repository)\n", path)
	fmt.Printf("public key, paste into cli/whisk/release_pubkey.go as ReleasePublicKey:\n%s\n", kp.Public)
	return nil
}

// pubkey prints the public half of the key in the file, the value release_pubkey.go holds.
func pubkey(keyPath string) error {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}
	pub, err := release.PublicKey(string(key))
	if err != nil {
		return err
	}
	fmt.Println(pub)
	return nil
}

// sign prints the detached signature of the file, the content of SHA256SUMS.sig.
func sign(keyPath, file string) error {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	sig, err := release.Sign(string(key), data)
	if err != nil {
		return err
	}
	fmt.Println(sig)
	return nil
}

// verify checks a signature file against the public key exactly as whisk update will, so a
// release whose embedded key or signature is wrong fails at build time instead of on users.
func verify(publicKey, file, sigFile string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	sig, err := os.ReadFile(sigFile)
	if err != nil {
		return err
	}
	ok, err := release.Verify(publicKey, data, string(sig))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no public key given")
	}
	return nil
}

package doctor

import (
	"bytes"
	"fmt"

	"github.com/whisk-run/cli/internal/stack"
	"github.com/whisk-run/contract/signinlook"
)

// w130 checks the logo sign_in names, as the push does: in the repository, a PNG or JPEG by its
// first bytes, at most 100 KB.
func w130(r Repo, _ Context) outcome {
	in := r.Manifest.SignIn
	if in == nil || in.Logo == "" {
		return outcome{}
	}
	line := yamlLine(r.ManifestNode, "/sign_in/logo")
	name := in.LogoPath()
	f, ok := r.file(name)
	if name == "" || !ok {
		return one("W130", manifestFile, line, fmt.Sprintf("sign_in.logo names %s, which is not in the repository.", in.Logo))
	}
	head := f.Head
	if head == nil {
		head = f.Content
	}
	switch {
	case f.Size > signinlook.MaxLogo:
		return one("W130", manifestFile, line, fmt.Sprintf("%s is %d KB; a sign-in logo is at most %d KB.", name, (f.Size+1023)/1024, signinlook.MaxLogo>>10))
	case signinlook.LogoType(head) == "":
		return one("W130", manifestFile, line, name+" is not a PNG or JPEG; several email programs show neither SVG nor WebP.")
	}
	return outcome{}
}

// w131 warns when sign_in's text and background are too close to read.
func w131(r Repo, _ Context) outcome {
	in := r.Manifest.SignIn
	if in == nil || (in.Text == "" && in.Background == "") {
		return outcome{}
	}
	text := signinlook.Normal(in.Text, signinlook.DefaultText)
	bg := signinlook.Normal(in.Background, signinlook.DefaultBackground)
	ratio := signinlook.Contrast(text, bg)
	if ratio >= signinlook.MinContrast {
		return outcome{}
	}
	at := "/sign_in/text"
	if in.Text == "" {
		at = "/sign_in/background"
	}
	return one("W131", manifestFile, yamlLine(r.ManifestNode, at), fmt.Sprintf("sign_in's text %s on background %s has a contrast of %.1f:1; customers need at least 4.5:1 to read it.", text, bg, ratio))
}

// signOutPaths are the addresses that give a customer a way out (CADDY.md §5.1).
var signOutPaths = [][]byte{[]byte("/.whisk/account"), []byte("/.whisk/logout")}

// w132 warns when an app with customers never links them a way to sign out.
func w132(r Repo, _ Context) outcome {
	if r.Manifest.CustomerIdentity != "app" && r.Manifest.CustomerIdentity != "org" {
		return outcome{}
	}
	for _, f := range r.Files {
		if f.Content == nil || f.Path == manifestFile || stack.IsTest(f.Path) || stack.IsVendored(f.Path) {
			continue
		}
		for _, p := range signOutPaths {
			if bytes.Contains(f.Content, p) {
				return outcome{}
			}
		}
	}
	return one("W132", manifestFile, yamlLine(r.ManifestNode, "/customer_identity"), fmt.Sprintf("customer_identity is %s, but nothing links /.whisk/account or /.whisk/logout, so a signed-in customer has no way to sign out.", r.Manifest.CustomerIdentity))
}

// file is the file at p, if the repository has it.
func (r Repo) file(p string) (File, bool) {
	for _, f := range r.Files {
		if f.Path == p {
			return f, true
		}
	}
	return File{}, false
}

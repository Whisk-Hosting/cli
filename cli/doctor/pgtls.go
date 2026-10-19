package doctor

import (
	"fmt"
	"regexp"

	"github.com/whisk-run/cli/internal/stack"
)

// W107 finds a node-postgres connection opened on the platform's address as it is. The
// address ends in sslmode=require, which pg reads as verify-full, so the connection is refused
// with "unable to get local issuer certificate" the first time it runs on Whisk, often in a
// background function long after the deploy went live (SKILL.md §5, doctor-rules.md).

var (
	// pgImport is a file using node-postgres directly or through Prisma's adapter.
	pgImport = regexp.MustCompile(`(?:\bfrom\s+["']pg["']|\brequire\(\s*["']pg["']\s*\)|["']@prisma/adapter-pg["'])`)
	// pgConnect is where that file opens a pool or a client.
	pgConnect = []*regexp.Regexp{regexp.MustCompile(`\bnew\s+(?:\w+\.)?(?:Pool|Client|PrismaPg)\s*\(`)}
	// pgTLSHandled is a file that sets libpq's meaning or the TLS options itself.
	pgTLSHandled = regexp.MustCompile(`\buselibpqcompat\b|\brejectUnauthorized\b|\bssl\s*:`)
)

func w107(r Repo, _ Context) outcome {
	var out outcome
	if r.Manifest.Database == "none" {
		return out
	}
	for _, f := range r.Code(stack.JS) {
		if !pgImport.Match(f.Content) || pgTLSHandled.Match(f.Content) {
			continue
		}
		if hs := matches(f, pgConnect, 0); len(hs) > 0 {
			h := earliest(hs)
			out = out.add("W107", f.Path, h.Line, fmt.Sprintf("%s line %d opens a node-postgres connection on the address as it is; pg reads its sslmode=require as verify-full and refuses the platform's certificate with \"unable to get local issuer certificate\".", f.Path, h.Line))
		}
	}
	return out
}

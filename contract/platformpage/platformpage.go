// Package platformpage draws the pages the platform itself answers a browser with when there is
// no app or dashboard page to show: the waking page, sign-in and access walls in front of an
// app, an app that is paused or unreachable, an address nothing answers, and API errors opened
// in a browser (CADDY.md §5, CONTROL-PLANE.md §9). They are in the Workbench Mustard design
// (DASHBOARD.md §2): warm white paper, navy ink, the dot-and-serif wordmark and one mustard
// accent. Each page is one self-contained document with its styles inline, because it is served
// from an app's own address where nothing else of Whisk's can be loaded.
package platformpage

import (
	"html"
	"strconv"
	"strings"
)

// Page is what one platform page shows.
type Page struct {
	// Heading is the sentence at the top, and the browser tab's title.
	Heading string
	// App names the app the page stands in front of; empty for the platform's own pages.
	App string
	// Lines are the paragraphs under the heading: what happened, then what to do.
	Lines []string
	// Code and RequestID go in the small print, for whoever reports the problem.
	Code      string
	RequestID string
	// Waiting shows the mustard spinner above the heading, for the waking page.
	Waiting bool
	// Offer is one line under the paragraphs that leads with a link, for the waking page's
	// upgrade line; nil leaves it out. Its URL must be an https address.
	Offer *Offer
	// Script is inline JavaScript run at the end of the page (the waking page's readiness poll),
	// and NoScript the line shown instead when scripts are off.
	Script   string
	NoScript string
	// Retry makes the page heal itself (RESILIENCE.md 1): a Retry button, and one reload on
	// its own; nil leaves both out.
	Retry *Retry
}

// Retry is how a page heals itself. The page reloads itself at most once within five minutes
// for the same address, remembered in the tab's session storage, or, where a browser keeps
// none, by the query flag __whisk_reload=1 on the reloaded address. After that it stays put
// and shows its sentence and the Retry button, so a page that keeps failing never spins or
// reloads for ever.
type Retry struct {
	// After is how many seconds the page waits before it reloads itself once; zero never
	// reloads on its own unless AfterJS is set.
	After int
	// AfterJS, when set, is a JavaScript expression for those seconds instead, for a page whose
	// server fills the value in (the edge quotes the response's Retry-After). It is clamped to
	// between 1 and 120 seconds, and 5 when it is not a number.
	AfterJS string
	// Hidden keeps the sentence and the button out of sight until the page's own Script gives
	// up by calling whiskHeal(0) (the waking page, which polls first).
	Hidden bool
	// Sentence is said beside the button; empty when the page's lines already say it.
	Sentence string
}

// ReloadFlag is the query parameter a page that cannot use session storage adds to the address
// it reloads, so it reloads only once.
const ReloadFlag = "__whisk_reload"

// healScript defines whiskHeal(seconds): reload the page once after seconds, or, when it has
// already done so for this address in the last five minutes, stop the spinner and show the
// Retry line. whiskGiveUp() does the second part alone.
const healScript = `var whiskKey='whisk_reload:'+location.pathname+location.search;` +
	`function whiskMay(){` +
	`if(/[?&]` + ReloadFlag + `=1(&|$)/.test(location.search))return '';` +
	`try{var s=window.sessionStorage,t=Number(s.getItem(whiskKey))||0;` +
	`if(Date.now()-t<300000)return '';` +
	`s.setItem(whiskKey,String(Date.now()));if(s.getItem(whiskKey))return 'reload';}catch(e){}` +
	`return 'flag';}` +
	`function whiskGiveUp(){var s=document.querySelector('.spin');if(s)s.style.display='none';` +
	`var r=document.getElementById('whisk-retry');if(r)r.hidden=false;}` +
	`function whiskHeal(after){var how=whiskMay();if(!how){whiskGiveUp();return;}` +
	`setTimeout(function(){if(how==='reload'){location.reload();return;}` +
	`var u=location.href.replace(/#.*$/,'');` +
	`location.replace(u+(u.indexOf('?')<0?'?':'&')+'` + ReloadFlag + `=1');},after*1000);}`

// script is the page's inline JavaScript: the healing functions and the reload when the page
// has a Retry, then its own Script.
func (p Page) script() string {
	r := p.Retry
	if r == nil {
		return p.Script
	}
	out := healScript
	switch {
	case r.AfterJS != "":
		out += `whiskHeal(Math.min(Math.max(parseInt(` + r.AfterJS + `,10)||5,1),120));`
	case r.After > 0:
		out += `whiskHeal(` + strconv.Itoa(r.After) + `);`
	}
	return out + p.Script
}

// Offer is a link followed by the rest of its sentence: "<Link> <Rest>".
type Offer struct {
	Link string
	URL  string
	Rest string
}

const style = `:root{color-scheme:light}*{box-sizing:border-box}` +
	`body{margin:0;min-height:100vh;display:flex;flex-direction:column;background:#faf8f3;color:#14213d;` +
	`font:16px/1.5 ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}` +
	`header,footer{padding:1.25rem 1.5rem}` +
	`.mark{display:inline-flex;align-items:center;gap:.45rem;color:#14213d;text-decoration:none;` +
	`font-family:Georgia,"Times New Roman",serif;font-size:1.3rem;letter-spacing:-.01em}` +
	`.dot{width:.6rem;height:.6rem;border-radius:50%;background:#e3a92b;display:inline-block}` +
	`main{flex:1;display:flex;align-items:center;justify-content:center;padding:1.5rem}` +
	`.card{width:100%;max-width:34rem;background:#fff;border:1px solid #e6e1d3;border-radius:10px;padding:2rem}` +
	`.kicker{margin:0 0 .5rem;font:500 .75rem/1.4 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;` +
	`letter-spacing:.08em;text-transform:uppercase;color:#8a5a00}` +
	`h1{margin:0 0 .75rem;font:400 1.75rem/1.2 Georgia,"Times New Roman",serif;letter-spacing:-.01em}` +
	`p{margin:.5rem 0;color:#4c5670}` +
	`.small{margin-top:1.25rem;padding-top:1rem;border-top:1px solid #e6e1d3;font-size:.8rem}` +
	`.offer{margin:1.25rem 0 0;padding:.75rem 1rem;background:#fdf6e3;border-left:3px solid #e3a92b;` +
	`border-radius:6px;color:#14213d;font-size:.95rem}.offer a{color:#8a5a00;font-weight:600}` +
	`code{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:.9em;color:#14213d}` +
	`.spin{width:2rem;height:2rem;margin:0 0 1.25rem;border:3px solid #f3e2b0;border-top-color:#e3a92b;` +
	`border-radius:50%;animation:s 1s linear infinite}@keyframes s{to{transform:rotate(360deg)}}` +
	`@media (prefers-reduced-motion:reduce){.spin{animation-duration:3s}}` +
	`footer{background:#14213d;color:#fff;font-size:.85rem}footer .mark{color:#fff;font-size:1.05rem}` +
	`.retry a{display:inline-block;min-height:2.75rem;margin-top:.5rem;padding:.6rem 1.25rem;` +
	`background:#14213d;color:#fff;border-radius:6px;text-decoration:none;font-weight:600}` +
	`.retry a:focus-visible{outline:3px solid #e3a92b;outline-offset:2px}` +
	`@media (max-width:480px){.card{padding:1.5rem}h1{font-size:1.5rem}}`

// Render draws a page as a complete HTML document; every value is escaped.
func Render(p Page) string {
	esc := html.EscapeString
	title := p.Heading
	if p.App != "" {
		title = p.Heading + " · " + p.App
	}
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8">`)
	b.WriteString(`<meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex">`)
	b.WriteString(`<title>` + esc(title) + `</title><style>` + style + `</style></head><body>`)
	b.WriteString(`<header><span class="mark"><span class="dot" aria-hidden="true"></span>whisk</span></header>`)
	b.WriteString(`<main><div class="card">`)
	if p.Waiting {
		b.WriteString(`<div class="spin" role="presentation"></div>`)
	}
	if p.App != "" {
		b.WriteString(`<p class="kicker">` + esc(p.App) + `</p>`)
	}
	b.WriteString(`<h1>` + esc(p.Heading) + `</h1>`)
	for _, l := range p.Lines {
		if l != "" {
			b.WriteString(`<p>` + esc(l) + `</p>`)
		}
	}
	if o := p.Offer; o != nil && o.Link != "" && strings.HasPrefix(o.URL, "https://") {
		b.WriteString(`<p class="offer"><a href="` + esc(o.URL) + `">` + esc(o.Link) + `</a>`)
		if o.Rest != "" {
			b.WriteString(` ` + esc(o.Rest))
		}
		b.WriteString(`</p>`)
	}
	if r := p.Retry; r != nil {
		b.WriteString(`<p class="retry" id="whisk-retry"`)
		if r.Hidden {
			b.WriteString(` hidden`)
		}
		b.WriteString(`>`)
		if r.Sentence != "" {
			b.WriteString(esc(r.Sentence) + `<br>`)
		}
		b.WriteString(`<a href="">Retry</a></p>`)
	}
	if p.NoScript != "" {
		b.WriteString(`<noscript><p>` + esc(p.NoScript) + `</p></noscript>`)
	}
	if small := smallPrint(p); small != "" {
		b.WriteString(`<p class="small">` + small + `</p>`)
	}
	b.WriteString(`</div></main>`)
	b.WriteString(`<footer>` + esc(hosted(p.App)) + `</footer>`)
	if js := p.script(); js != "" {
		b.WriteString(`<script>` + js + `</script>`)
	}
	b.WriteString(`</body></html>` + "\n")
	return b.String()
}

func smallPrint(p Page) string {
	var parts []string
	if p.Code != "" {
		parts = append(parts, `<code>`+html.EscapeString(p.Code)+`</code>`)
	}
	if p.RequestID != "" {
		parts = append(parts, `request <code>`+html.EscapeString(p.RequestID)+`</code>`)
	}
	return strings.Join(parts, " · ")
}

func hosted(app string) string {
	if app == "" {
		return "Whisk hosts business apps built with AI."
	}
	return app + " is hosted on Whisk."
}

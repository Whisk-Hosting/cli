// Package manifest parses and validates whisk.yaml against the schema for its conventions
// version and the manifest rules that go beyond types. Parse is a pure function of the bytes;
// on failure it lists every problem so all can be fixed in one pass.
package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"gopkg.in/yaml.v3"

	"github.com/whisk-run/contract"
	"github.com/whisk-run/contract/connect"
	"github.com/whisk-run/contract/redirects"
	"github.com/whisk-run/contract/routes"
	"github.com/whisk-run/contract/webhook"
)

// Manifest is a parsed, validated whisk.yaml with every default applied.
type Manifest struct {
	Whisk         int               `json:"whisk"`
	Name          string            `json:"name"`
	Routes        Routes            `json:"routes"`
	Health        Health            `json:"health"`
	Database      string            `json:"database"`
	Migrate       string            `json:"migrate,omitempty"`
	DatabaseRole  string            `json:"database_role"`
	Build         Build             `json:"build"`
	Secrets       []string          `json:"secrets"`
	Env           map[string]string `json:"env"`
	Queue         Queue             `json:"queue"`
	Functions     []Function        `json:"functions"`
	Webhooks      []Webhook         `json:"webhooks"`
	Inbox         *Inbox            `json:"inbox,omitempty"`
	Static        []Static          `json:"static"`
	Redirects     []redirects.Rule  `json:"redirects"`
	RedirectsFile string            `json:"redirects_file,omitempty"`
	Storage       bool              `json:"storage"`
	KV            bool              `json:"kv"`
	Email         bool              `json:"email"`
	AlwaysOn      bool              `json:"always_on"`
	Calls         []string          `json:"calls"`
	// Connections are outside systems reached through the broker (CONTRACT.md §3.1).
	Connections      map[string]connect.Connection `json:"connections"`
	CustomerIdentity string                        `json:"customer_identity"`
	// Network is where the app is served: NetworkInternal or NetworkPublic, or empty for the
	// edition's default (public on whisk.run, internal on Whisk On-Premise).
	Network  string   `json:"network,omitempty"`
	Previews Previews `json:"previews"`
	// Egress is EgressOpen or EgressClosed (MANAGED-APPS.md §6).
	Egress string `json:"egress"`
	// Managed declares the product this app is the source of (MANAGED-APPS.md §2); nil for
	// every other app.
	Managed *Managed `json:"managed,omitempty"`
}

// Managed is a product's declaration in its source app's manifest (MANAGED-APPS.md §2).
type Managed struct {
	Product  string             `json:"product"`
	Name     string             `json:"name"`
	Settings []string           `json:"settings"`
	Links    []string           `json:"links"`
	Variants map[string]Variant `json:"variants"`
}

// Variant is one form a product comes in: what a copy of it adds to the release manifest.
type Variant struct {
	Name string            `json:"name"`
	Env  map[string]string `json:"env"`
	// Settings are names a business may set on a copy of this variant, besides the product's.
	Settings    []string                      `json:"settings,omitempty"`
	Connections map[string]connect.Connection `json:"connections,omitempty"`
}

// Egress values (MANAGED-APPS.md §6).
const (
	EgressOpen   = "open"
	EgressClosed = "closed"
)

type Routes struct {
	Public []string `json:"public"`
	// Apps are routes only other apps' service calls reach (MANAGED-APPS.md §5).
	Apps      []string          `json:"apps"`
	Challenge []string          `json:"challenge"`
	CSRFOff   []string          `json:"csrf_off"`
	Headers   map[string]string `json:"headers"`
}

type Health struct {
	Path    string `json:"path"`
	Timeout int    `json:"timeout"`
}

type Build struct {
	Dockerfile string   `json:"dockerfile,omitempty"`
	Secrets    []string `json:"secrets"`
}

type Queue struct {
	Endpoint string `json:"endpoint"`
}

type Function struct {
	Name        string `json:"name"`
	Cron        string `json:"cron,omitempty"`
	TZ          string `json:"tz,omitempty"`
	Event       string `json:"event,omitempty"`
	Graph       string `json:"graph"`
	Concurrency int    `json:"concurrency,omitempty"`
	Retries     int    `json:"retries"`
}

type Webhook struct {
	Name        string          `json:"name"`
	Preset      string          `json:"preset"`
	Secret      string          `json:"secret,omitempty"`
	Handler     string          `json:"handler"`
	IPAllowlist []string        `json:"ip_allowlist"`
	HMAC        *webhook.Preset `json:"hmac,omitempty"`
	// Handshake, when set, answers the provider's check of the address (webhook.Handshake).
	Handshake *webhook.Handshake `json:"handshake,omitempty"`
}

// Inbox is the app's inbound email (CONTRACT.md §7, "Inbound email"): every message to the app's
// address is stored and delivered to Handler as JSON. AllowFrom, when set, names the only senders
// that reach it: whole addresses, or "@domain" for every address at that domain.
type Inbox struct {
	Handler   string   `json:"handler"`
	AllowFrom []string `json:"allow_from"`
}

// Whose pool an app's customers belong to (CONTROL-PLANE.md §4.6): none, its own, or one
// shared with the org's other apps that ask for the same.
const (
	CustomerIdentityNone = "none"
	CustomerIdentityApp  = "app"
	CustomerIdentityOrg  = "org"
)

// Which login an app's own code connects as (CONTRACT.md §8): the role that owns its tables, or
// a run login that owns nothing while only the migrate step connects as the owner.
const (
	DatabaseRoleOwner      = "owner"
	DatabaseRoleRestricted = "restricted"
)

// Restricted reports whether the app runs as its database's run login. A manifest stored before
// the field existed reads as the owner.
func (m Manifest) Restricted() bool { return m.DatabaseRole == DatabaseRoleRestricted }

// Network values (ON-PREMISE.md §5): internal serves the app only on the edge's inside address,
// public on the outside address too.
const (
	NetworkInternal = "internal"
	NetworkPublic   = "public"
)

type Static struct {
	Dir  string `json:"dir"`
	Path string `json:"path"`
}

type Previews struct {
	Database string `json:"database"`
	TTLDays  int    `json:"ttl_days"`
}

// Problem is one validation finding. Path is a JSON pointer into the manifest ("/functions/0").
// Code is MANIFEST_INVALID, MANIFEST_UNKNOWN_KEY or CONVENTIONS_VERSION_UNSUPPORTED.
type Problem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

func (p Problem) String() string { return p.Path + ": " + p.Message }

// Problems is the error returned by Parse when validation fails.
type Problems []Problem

func (ps Problems) Error() string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = p.String()
	}
	return "whisk.yaml has " + strconv.Itoa(len(ps)) + " problem(s): " + strings.Join(parts, "; ")
}

// Code returns the code that describes the whole result: CONVENTIONS_VERSION_UNSUPPORTED or
// MANIFEST_UNKNOWN_KEY when every problem is of that kind, otherwise MANIFEST_INVALID.
func (ps Problems) Code() string {
	if len(ps) == 0 {
		return ""
	}
	first := ps[0].Code
	for _, p := range ps {
		if p.Code != first {
			return "MANIFEST_INVALID"
		}
	}
	return first
}

// ReservedEnvNames may not be declared under secrets, env or build.secrets.
var ReservedEnvNames = map[string]bool{"PORT": true, "DATABASE_URL": true, "SENTRY_DSN": true, "TZ": true}

var (
	compiledSchema = mustCompile("whisk.schema.json", contract.ManifestSchema)
	printer        = message.NewPrinter(language.English)
	knownKeys      = topLevelKeys()
	reFunctionPath = regexp.MustCompile(`^/functions/\d+$`)
	reWebhookPath  = regexp.MustCompile(`^/webhooks/\d+$`)
)

// Schema returns the compiled manifest schema.
func Schema() *jsonschema.Schema { return compiledSchema }

// Parse decodes and validates whisk.yaml. On success every default is applied. On failure the
// error is a Problems value listing everything wrong.
func Parse(src []byte) (Manifest, error) {
	var raw any
	if err := yaml.Unmarshal(src, &raw); err != nil {
		return Manifest{}, Problems{{Path: "", Message: "does not parse as YAML: " + err.Error(), Code: "MANIFEST_INVALID"}}
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return Manifest{}, Problems{{Path: "", Message: "the file must be a YAML mapping with whisk: 1 and name:", Code: "MANIFEST_INVALID"}}
	}
	if ps := checkVersion(obj); ps != nil {
		return Manifest{}, ps
	}
	if ps := validateSchema(obj); ps != nil {
		return Manifest{}, ps
	}
	m, err := decode(withRawDefaults(obj))
	if err != nil {
		return Manifest{}, Problems{{Path: "", Message: err.Error(), Code: "MANIFEST_INVALID"}}
	}
	m = withDefaults(m)
	if ps := checkRules(m); ps != nil {
		return Manifest{}, ps
	}
	return m, nil
}

func checkVersion(obj map[string]any) Problems {
	v, present := obj["whisk"]
	if !present {
		return Problems{{Path: "/whisk", Message: "required; set whisk: 1", Code: "MANIFEST_INVALID"}}
	}
	n, ok := v.(int)
	if !ok || n != contract.Version {
		return Problems{{Path: "/whisk", Message: fmt.Sprintf("declares conventions version %v; this contract serves version %d", v, contract.Version), Code: "CONVENTIONS_VERSION_UNSUPPORTED"}}
	}
	return nil
}

func mustCompile(name string, src []byte) *jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(src))
	if err != nil {
		panic(name + ": " + err.Error())
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(name, doc); err != nil {
		panic(name + ": " + err.Error())
	}
	return c.MustCompile(name)
}

func validateSchema(obj map[string]any) Problems {
	err := compiledSchema.Validate(obj)
	if err == nil {
		return nil
	}
	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return Problems{{Path: "", Message: err.Error(), Code: "MANIFEST_INVALID"}}
	}
	return flatten(ve, obj)
}

// explainConditional turns a failed oneOf/not at a function or webhook into the rule it
// encodes, because the schema's own wording ("'not' failed") does not say what to change.
func explainConditional(path string, obj map[string]any, fallback string) string {
	switch {
	case reFunctionPath.MatchString(path):
		return "needs exactly one of cron or event"
	case reWebhookPath.MatchString(path):
		w := at(obj, path)
		preset, _ := w["preset"].(string)
		_, hasSecret := w["secret"]
		_, hasHMAC := w["hmac"]
		switch {
		case preset == webhook.PresetToken && hasSecret:
			return "a token webhook has no signing secret; remove secret"
		case preset == webhook.PresetHMAC && !hasHMAC:
			return "preset hmac needs its settings under hmac (header, algorithm, encoding, payload)"
		case preset != webhook.PresetHMAC && hasHMAC:
			return "hmac settings only apply to preset hmac"
		case preset != webhook.PresetToken && !hasSecret:
			return "secret is required: the name of the secret holding the signing secret"
		}
	}
	return fallback
}

// at resolves a JSON pointer of the shape /key/index into the raw manifest.
func at(obj map[string]any, path string) map[string]any {
	segs := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(segs) != 2 {
		return nil
	}
	list, _ := obj[segs[0]].([]any)
	i, err := strconv.Atoi(segs[1])
	if err != nil || i < 0 || i >= len(list) {
		return nil
	}
	m, _ := list[i].(map[string]any)
	return m
}

// flatten turns the nested validation error into leaf problems with JSON pointers. Unknown
// keys become MANIFEST_UNKNOWN_KEY with the nearest known key suggested.
func flatten(ve *jsonschema.ValidationError, obj map[string]any) Problems {
	var out Problems
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) > 0 {
			for _, c := range e.Causes {
				walk(c)
			}
			return
		}
		path := "/" + strings.Join(e.InstanceLocation, "/")
		if path == "/" {
			path = ""
		}
		switch k := e.ErrorKind.(type) {
		case *kind.AdditionalProperties:
			for _, key := range k.Properties {
				p := Problem{Path: path + "/" + key, Message: "unknown key " + key, Code: "MANIFEST_UNKNOWN_KEY"}
				if s := nearest(key, siblingKeys(path)); s != "" {
					p.Message += "; did you mean " + s
				}
				out = append(out, p)
			}
		case *kind.OneOf, *kind.Not:
			out = append(out, Problem{Path: path, Message: explainConditional(path, obj, e.ErrorKind.LocalizedString(printer)), Code: "MANIFEST_INVALID"})
		default:
			out = append(out, Problem{Path: path, Message: e.ErrorKind.LocalizedString(printer), Code: "MANIFEST_INVALID"})
		}
	}
	walk(ve)
	return dedupe(out)
}

func dedupe(ps Problems) Problems {
	seen := map[string]bool{}
	var out Problems
	for _, p := range ps {
		key := p.Path + "\x00" + p.Message
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// siblingKeys returns the known keys at a path: the top level, or a nested object's keys
// looked up in the schema.
func siblingKeys(path string) []string {
	if path == "" {
		return knownKeys
	}
	var s map[string]any
	_ = json.Unmarshal(contract.ManifestSchema, &s)
	node := s
	for _, seg := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if _, err := strconv.Atoi(seg); err == nil {
			node = resolveRef(s, node["items"])
			continue
		}
		props, _ := node["properties"].(map[string]any)
		node = resolveRef(s, props[seg])
		if node == nil {
			return nil
		}
	}
	props, _ := node["properties"].(map[string]any)
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func resolveRef(root map[string]any, v any) map[string]any {
	node, _ := v.(map[string]any)
	ref, _ := node["$ref"].(string)
	if !strings.HasPrefix(ref, "#/$defs/") {
		return node
	}
	defs, _ := root["$defs"].(map[string]any)
	target, _ := defs[strings.TrimPrefix(ref, "#/$defs/")].(map[string]any)
	return target
}

func topLevelKeys() []string {
	var s struct {
		Properties map[string]any `json:"properties"`
	}
	_ = json.Unmarshal(contract.ManifestSchema, &s)
	keys := make([]string, 0, len(s.Properties))
	for k := range s.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// nearest returns the known key within edit distance 2 of k, or "".
func nearest(k string, known []string) string {
	best, bestD := "", 3
	for _, c := range known {
		if d := levenshtein(k, c); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

// withRawDefaults fills defaults the typed decode cannot distinguish from zero values.
func withRawDefaults(obj map[string]any) map[string]any {
	fns, _ := obj["functions"].([]any)
	if len(fns) == 0 {
		return obj
	}
	out := make(map[string]any, len(obj))
	for k, v := range obj {
		out[k] = v
	}
	filled := make([]any, len(fns))
	for i, f := range fns {
		fm, _ := f.(map[string]any)
		c := make(map[string]any, len(fm)+1)
		for k, v := range fm {
			c[k] = v
		}
		if _, ok := c["retries"]; !ok {
			c["retries"] = 3
		}
		filled[i] = c
	}
	out["functions"] = filled
	return out
}

func decode(obj map[string]any) (Manifest, error) {
	b, err := json.Marshal(obj)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func withDefaults(m Manifest) Manifest {
	m.Routes.Public = orEmpty(m.Routes.Public)
	m.Routes.Challenge = orEmpty(m.Routes.Challenge)
	m.Routes.CSRFOff = orEmpty(m.Routes.CSRFOff)
	m.Routes.Apps = orEmpty(m.Routes.Apps)
	if m.Egress == "" {
		m.Egress = EgressOpen
	}
	if m.Managed != nil {
		m.Managed.Settings = orEmpty(m.Managed.Settings)
		m.Managed.Links = orEmpty(m.Managed.Links)
		if m.Managed.Variants == nil {
			m.Managed.Variants = map[string]Variant{}
		}
		for k, v := range m.Managed.Variants {
			if v.Env == nil {
				v.Env = map[string]string{}
			}
			m.Managed.Variants[k] = v
		}
	}
	if m.Routes.Headers == nil {
		m.Routes.Headers = map[string]string{}
	}
	if m.Health.Path == "" {
		m.Health.Path = "/health"
	}
	if m.Health.Timeout == 0 {
		m.Health.Timeout = 90
	}
	if m.Database == "" {
		m.Database = "app"
	}
	if m.DatabaseRole == "" {
		m.DatabaseRole = DatabaseRoleOwner
	}
	m.Build.Secrets = orEmpty(m.Build.Secrets)
	m.Secrets = orEmpty(m.Secrets)
	if m.Env == nil {
		m.Env = map[string]string{}
	}
	if m.Queue.Endpoint == "" {
		m.Queue.Endpoint = "/.whisk/inngest"
	}
	if m.Functions == nil {
		m.Functions = []Function{}
	}
	if m.Webhooks == nil {
		m.Webhooks = []Webhook{}
	}
	for i := range m.Webhooks {
		m.Webhooks[i].IPAllowlist = orEmpty(m.Webhooks[i].IPAllowlist)
	}
	if m.Inbox != nil {
		m.Inbox.AllowFrom = orEmpty(m.Inbox.AllowFrom)
	}
	if m.Static == nil {
		m.Static = []Static{}
	}
	if m.Redirects == nil {
		m.Redirects = []redirects.Rule{}
	}
	m.Calls = orEmpty(m.Calls)
	conns := make(map[string]connect.Connection, len(m.Connections))
	for name, c := range m.Connections {
		conns[name] = c.WithDefaults()
	}
	m.Connections = conns
	if m.CustomerIdentity == "" {
		m.CustomerIdentity = CustomerIdentityNone
	}
	if m.Previews.Database == "" {
		m.Previews.Database = "empty"
	}
	if m.Previews.TTLDays == 0 {
		m.Previews.TTLDays = 3
	}
	return m
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func checkRules(m Manifest) Problems {
	var ps Problems
	add := func(path, msg string) { ps = append(ps, Problem{Path: path, Message: msg, Code: "MANIFEST_INVALID"}) }

	for i, r := range m.Routes.Challenge {
		if !routes.MatchAny(m.Routes.Public, r) && !contains(m.Routes.Public, r) {
			add(fmt.Sprintf("/routes/challenge/%d", i), r+" is not a public route; challenge routes must also be listed under routes.public")
		}
	}
	for i, n := range m.Secrets {
		if msg := reservedName(n); msg != "" {
			add(fmt.Sprintf("/secrets/%d", i), msg)
		}
	}
	for i, r := range m.Routes.Apps {
		if routes.MatchAny(m.Routes.Public, r) || contains(m.Routes.Public, r) {
			add(fmt.Sprintf("/routes/apps/%d", i), r+" is also a public route; a route only apps may call cannot be public")
		}
	}
	ps = append(ps, checkManaged(m)...)
	for i, n := range m.Build.Secrets {
		if msg := reservedName(n); msg != "" {
			add(fmt.Sprintf("/build/secrets/%d", i), msg)
		}
	}
	for _, k := range sortedKeys(m.Env) {
		if msg := reservedName(k); msg != "" {
			add("/env/"+k, msg)
		}
	}
	seen := map[string]int{}
	for i, f := range m.Functions {
		if j, dup := seen[f.Name]; dup {
			add(fmt.Sprintf("/functions/%d/name", i), fmt.Sprintf("duplicate function name %s (also /functions/%d)", f.Name, j))
		}
		seen[f.Name] = i
		if f.Cron != "" {
			if err := CheckCron(f.Cron); err != nil {
				add(fmt.Sprintf("/functions/%d/cron", i), err.Error())
			}
		}
		if f.TZ != "" {
			if _, err := time.LoadLocation(f.TZ); err != nil {
				add(fmt.Sprintf("/functions/%d/tz", i), f.TZ+" is not a known IANA time zone")
			}
			if f.Cron == "" {
				add(fmt.Sprintf("/functions/%d/tz", i), "tz only applies to cron functions")
			}
		}
	}
	if m.Restricted() {
		if m.Database == "none" {
			add("/database_role", "database_role: restricted needs a database; remove it or declare database: app or shared:<name>")
		}
		if m.Migrate == "" {
			add("/database_role", "database_role: restricted needs a migrate command: only the migrate step connects as the owner that can create and change tables")
		}
	}
	presets := webhook.Presets()
	seenHooks := map[string]int{}
	for i, w := range m.Webhooks {
		if j, dup := seenHooks[w.Name]; dup {
			add(fmt.Sprintf("/webhooks/%d/name", i), fmt.Sprintf("duplicate webhook name %s (also /webhooks/%d)", w.Name, j))
		}
		seenHooks[w.Name] = i
		if _, known := presets[w.Preset]; !known {
			add(fmt.Sprintf("/webhooks/%d/preset", i), w.Preset+" is not a known preset; see webhook-presets.yaml")
		}
		if w.Secret != "" && !contains(m.Secrets, w.Secret) {
			add(fmt.Sprintf("/webhooks/%d/secret", i), w.Secret+" is not listed under secrets")
		}
		if w.HMAC != nil {
			if err := w.HMAC.Check(); err != nil {
				add(fmt.Sprintf("/webhooks/%d/hmac", i), err.Error())
			}
		}
	}
	held := map[string]string{}
	for i, n := range m.Secrets {
		held[n] = fmt.Sprintf("/secrets/%d", i)
	}
	for i, n := range m.Build.Secrets {
		held[n] = fmt.Sprintf("/build/secrets/%d", i)
	}
	for _, name := range sortedConnections(m.Connections) {
		c := m.Connections[name]
		base := "/connections/" + name
		if !connect.Name.MatchString(name) {
			add(base, name+" is not a connection name: lower case letters, digits and _ , starting with a letter, up to 30")
		}
		found, secrets := connect.Check(c)
		for _, p := range found {
			add(base+p.Path, p.Message)
		}
		for _, n := range secrets {
			if msg := reservedName(n); msg != "" {
				add(base+"/auth", msg)
			}
			if where, ok := held[n]; ok {
				add(base+"/auth", fmt.Sprintf("%s is also listed at %s, which would give the app the value; a connection's keys stay with Whisk, so remove it there", n, where))
			}
		}
	}
	if m.Inbox != nil {
		if i, clash := seenHooks[InboxSource]; clash {
			add(fmt.Sprintf("/webhooks/%d/name", i), "the webhook name inbox is the inbox's own while inbox: is declared; rename the source")
		}
		if m.Inbox.Handler == m.Queue.Endpoint {
			add("/inbox/handler", "the inbox handler cannot be the queue endpoint")
		}
	}
	for _, p := range redirects.Check(m.Redirects) {
		add(fmt.Sprintf("/redirects/%d/%s", p.Index, p.Field), p.Message)
	}
	if len(ps) == 0 {
		return nil
	}
	return dedupe(ps)
}

func sortedConnections(m map[string]connect.Connection) []string {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ConnectionSecrets is every secret name the manifest's connections read, sorted.
func ConnectionSecrets(m Manifest) []string {
	seen := map[string]bool{}
	var out []string
	for _, name := range sortedConnections(m.Connections) {
		for _, n := range connect.Secrets(m.Connections[name]) {
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	sort.Strings(out)
	return out
}

// WithFile returns the manifest with the redirects file's rules after its own, so the
// manifest recorded for a commit carries every rule the edge serves (CONTRACT.md §3.1). The
// problems name the file's lines, and the rules checked together: a duplicate or a loop
// across the two, or too many in all.
func (m Manifest) WithFile(src []byte) (Manifest, []string) {
	parsed, bad := redirects.ParseFile(src)
	var out []string
	for _, p := range bad {
		out = append(out, fmt.Sprintf("%s %s", m.RedirectsFile, p))
	}
	if len(out) > 0 {
		return m, out
	}
	all := append(append([]redirects.Rule{}, m.Redirects...), parsed.Rules...)
	for _, p := range redirects.Check(all) {
		if i := p.Index - len(m.Redirects); i >= 0 && i < len(parsed.Lines) {
			out = append(out, fmt.Sprintf("%s line %d: %s %s", m.RedirectsFile, parsed.Lines[i], p.Field, p.Message))
		} else {
			out = append(out, fmt.Sprintf("redirects[%d].%s: %s", p.Index, p.Field, p.Message))
		}
	}
	m.Redirects = all
	return m, out
}

// checkManaged checks a product's declaration: its settings, variants' settings and variants'
// env are names the app may set, a setting is not a secret, a variant's setting is neither the
// product's nor fixed by its own env, and no variant's env sets a product setting.
func checkManaged(m Manifest) Problems {
	if m.Managed == nil {
		return nil
	}
	var ps Problems
	add := func(path, msg string) { ps = append(ps, Problem{Path: path, Message: msg, Code: "MANIFEST_INVALID"}) }
	for i, n := range m.Managed.Settings {
		if msg := reservedName(n); msg != "" {
			add(fmt.Sprintf("/managed/settings/%d", i), msg)
		}
		if contains(m.Secrets, n) {
			add(fmt.Sprintf("/managed/settings/%d", i), n+" is a secret; a setting is a plain value the business sets on its copy")
		}
	}
	for _, name := range sortedVariants(m.Managed.Variants) {
		v := m.Managed.Variants[name]
		for i, n := range v.Settings {
			path := fmt.Sprintf("/managed/variants/%s/settings/%d", name, i)
			if msg := reservedName(n); msg != "" {
				add(path, msg)
			}
			if contains(m.Secrets, n) {
				add(path, n+" is a secret; a setting is a plain value the business sets on its copy")
			}
			if contains(m.Managed.Settings, n) {
				add(path, n+" is already one of the product's settings")
			}
			if _, fixed := v.Env[n]; fixed {
				add(path, n+" is fixed by the variant's env; a setting is set by the business")
			}
		}
		for _, cn := range sortedConnections(v.Connections) {
			base := "/managed/variants/" + name + "/connections/" + cn
			if !connect.Name.MatchString(cn) {
				add(base, cn+" is not a connection name: lower case letters, digits and _ , starting with a letter, up to 30")
			}
			c := v.Connections[cn]
			placeholders := map[string]string{}
			for _, n := range connect.Settings(c) {
				if !contains(m.Managed.Settings, n) && !contains(v.Settings, n) {
					add(base, "{setting."+n+"} names no setting of the product or this variant")
				}
				placeholders[n] = connect.SettingPlaceholder
			}
			filled, err := connect.FillSettings(c, placeholders)
			if err != nil {
				add(base, err.Error())
				continue
			}
			found, _ := connect.Check(filled.WithDefaults())
			for _, p := range found {
				add(base+p.Path, p.Message)
			}
		}
		for _, k := range sortedKeys(m.Managed.Variants[name].Env) {
			path := "/managed/variants/" + name + "/env/" + k
			if msg := reservedName(k); msg != "" {
				add(path, msg)
			}
			if contains(m.Managed.Settings, k) {
				add(path, k+" is a setting the business sets; a variant cannot also fix it")
			}
		}
	}
	return ps
}

func sortedVariants(m map[string]Variant) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// IsApps reports whether path is a route only other apps may call.
func (m Manifest) IsApps(path string) bool { return routes.MatchAny(m.Routes.Apps, path) }

func reservedName(n string) string {
	switch {
	case strings.HasPrefix(n, "WHISK_"):
		return n + " begins with WHISK_, which is reserved for the platform"
	case ReservedEnvNames[n]:
		return n + " is set by the platform and cannot be declared"
	}
	return ""
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// InboxSource is the webhook source an inbox's messages are stored and replayed under.
const InboxSource = "inbox"

// ServiceRoutes returns the paths only the platform may call: the queue endpoint, every
// webhook handler and the inbox handler.
func (m Manifest) ServiceRoutes() []string {
	out := []string{m.Queue.Endpoint}
	for _, w := range m.Webhooks {
		out = append(out, w.Handler)
	}
	if m.Inbox != nil {
		out = append(out, m.Inbox.Handler)
	}
	return out
}

// IsPublic reports whether path is reachable without identity.
func (m Manifest) IsPublic(path string) bool { return routes.MatchAny(m.Routes.Public, path) }

// IsService reports whether path is a service route.
func (m Manifest) IsService(path string) bool {
	return contains(m.ServiceRoutes(), strings.TrimSuffix(path, "/")) || contains(m.ServiceRoutes(), path)
}

// NeedsChallenge reports whether a POST to path must carry a solved challenge.
func (m Manifest) NeedsChallenge(path string) bool { return routes.MatchAny(m.Routes.Challenge, path) }

// CSRFExempt reports whether path is listed under routes.csrf_off.
func (m Manifest) CSRFExempt(path string) bool { return routes.MatchAny(m.Routes.CSRFOff, path) }

// FunctionByName returns the declared function with that name.
func (m Manifest) FunctionByName(name string) (Function, bool) {
	for _, f := range m.Functions {
		if f.Name == name {
			return f, true
		}
	}
	return Function{}, false
}

// WebhookByName returns the declared webhook source with that name.
func (m Manifest) WebhookByName(name string) (Webhook, bool) {
	for _, w := range m.Webhooks {
		if w.Name == name {
			return w, true
		}
	}
	return Webhook{}, false
}

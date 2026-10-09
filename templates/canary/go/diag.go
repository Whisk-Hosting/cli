package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// The diagnostics the platform's canary drives to prove the container's boundaries
// (HARNESS.md H18, H22, H37 and H42). Every route is private, answers JSON, and is safe to delete in
// your own app. They are the same in the three templates.

// internalPort is the edge's internal listener, where app-to-app calls arrive
// (CADDY.md §4.3).
const internalPort = "8443"

// diagReport is what GET /diag answers: the process user, whether the root filesystem and
// /tmp take writes, the container's own addresses, whether the host's Docker socket is
// visible, and the database role and name the app connects as.
func diagReport() map[string]any {
	role, database := dbIdentity(os.Getenv("DATABASE_URL"))
	_, socketErr := os.Stat("/var/run/docker.sock")
	return map[string]any{
		"uid":           os.Getuid(),
		"root_writable": writable("/"),
		"tmp_writable":  writable("/tmp"),
		"addrs":         ownAddrs(),
		"docker_socket": socketErr == nil,
		"db_role":       role,
		"db_name":       database,
	}
}

// ownAddrs lists the container's IPv4 addresses in CIDR form, loopback left out.
func ownAddrs() []string {
	out := []string{}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return out
	}
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok || ipn.IP.To4() == nil || ipn.IP.IsLoopback() {
			continue
		}
		out = append(out, ipn.String())
	}
	return out
}

// dbIdentity reads the role and database out of a connection string.
func dbIdentity(raw string) (role, database string) {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return "", ""
	}
	return u.User.Username(), strings.TrimPrefix(u.Path, "/")
}

// diagCall is GET /diag/call?to=<app id>[&token=none][&via=<app id>]: one request to the
// named app's internal name with this app's service identity (or none), reporting the status
// the edge answered and the caller the target app saw. Only the apps this app declares in
// `calls` resolve from inside the container; `via` names one of them, and the request is sent
// to its address with `to` as the host, which is how the edge's refusal of an undeclared
// target is observed.
func diagCall(w http.ResponseWriter, req *http.Request) {
	to := req.URL.Query().Get("to")
	if to == "" {
		writeJSON(w, 400, map[string]string{"error": "to is required: the app id to call"})
		return
	}
	dial := to
	if via := req.URL.Query().Get("via"); via != "" {
		dial = via
	}
	target := "http://" + dial + ".internal.whisk:" + internalPort + "/whoami"
	ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
	defer cancel()
	out, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	out.Host = to + ".internal.whisk:" + internalPort
	out.Header.Set("Accept", "application/json")
	if req.URL.Query().Get("token") != "none" {
		out.Header.Set("Authorization", "Bearer "+os.Getenv("WHISK_SERVICE_TOKEN"))
	}
	resp, err := http.DefaultClient.Do(out)
	if err != nil {
		writeJSON(w, 200, map[string]any{"to": to, "url": target, "status": 0, "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	echoed := map[string]any{}
	_ = json.Unmarshal(body, &echoed)
	serviceApp, _ := echoed["x-whisk-service-app"].(string)
	audience, _ := echoed["x-whisk-audience"].(string)
	writeJSON(w, 200, map[string]any{"to": to, "url": target, "status": resp.StatusCode, "service_app": serviceApp, "audience": audience, "code": errorCode(body)})
}

// diagPost is POST /diag/call with a JSON body {app_id, path, headers, body}: one POST to the
// named app's internal name at path, carrying this app's service token, the given headers
// (a map of name to value, optional) and the body as sent. It answers 200 with what came
// back, {status, body, headers}, or {status: 0, error} when the call never completed. The
// harness uses it to show that the delivery headers an app forges on an app-to-app call
// are stripped and that a handler refuses the call. The GET form above is unchanged.
func diagPost(w http.ResponseWriter, req *http.Request) {
	var in struct {
		AppID   string            `json:"app_id"`
		Path    string            `json:"path"`
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}
	if err := json.NewDecoder(req.Body).Decode(&in); err != nil || in.AppID == "" || !strings.HasPrefix(in.Path, "/") {
		writeJSON(w, 400, map[string]string{"error": "a JSON body with app_id and a path starting with / is required"})
		return
	}
	target := "http://" + in.AppID + ".internal.whisk:" + internalPort + in.Path
	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()
	out, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(in.Body))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	out.Header.Set("Authorization", "Bearer "+os.Getenv("WHISK_SERVICE_TOKEN"))
	for name, value := range in.Headers {
		out.Header.Set(name, value)
	}
	resp, err := http.DefaultClient.Do(out)
	if err != nil {
		writeJSON(w, 200, map[string]any{"status": 0, "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	headers := map[string]string{}
	for name := range resp.Header {
		headers[strings.ToLower(name)] = resp.Header.Get(name)
	}
	writeJSON(w, 200, map[string]any{"status": resp.StatusCode, "body": string(body), "headers": headers})
}

// errorCode reads the platform error code out of a body, "" when there is none.
func errorCode(body []byte) string {
	var wrapped struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &wrapped)
	return wrapped.Error.Code
}

// diagPG is GET /diag/pg?role=<role>&database=<name>: a connection to the app's own database
// endpoint as another role or to another database, which the platform must refuse.
func diagPG(w http.ResponseWriter, req *http.Request) {
	u, err := url.Parse(os.Getenv("DATABASE_URL"))
	if err != nil || u.User == nil {
		writeJSON(w, 500, map[string]string{"error": "DATABASE_URL is not a URL"})
		return
	}
	role, database := u.User.Username(), strings.TrimPrefix(u.Path, "/")
	if r := req.URL.Query().Get("role"); r != "" {
		role = r
	}
	if d := req.URL.Query().Get("database"); d != "" {
		database = d
	}
	password, _ := u.User.Password()
	u.User = url.UserPassword(role, password)
	u.Path = "/" + database
	ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, u.String())
	if err != nil {
		writeJSON(w, 200, map[string]any{"role": role, "database": database, "connected": false, "error": err.Error()})
		return
	}
	defer conn.Close(ctx)
	var one int
	err = conn.QueryRow(ctx, "select 1").Scan(&one)
	writeJSON(w, 200, map[string]any{"role": role, "database": database, "connected": err == nil, "error": errText(err)})
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// shareRole is the shape of an app's database role, and so of its schema in a shared
// database (CONTRACT.md §8).
var shareRole = regexp.MustCompile(`^app_[a-z0-9_]{1,59}$`)

// diagConn is one connection to the app's own database, sending every query with its
// parameters because DATABASE_URL is pooled in transaction mode.
func diagConn(ctx context.Context) (*pgx.Conn, error) {
	cfg, err := pgx.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		return nil, err
	}
	cfg.DefaultQueryExecMode = pgx.QueryExecModeExec
	return pgx.ConnectConfig(ctx, cfg)
}

// diagShare is POST /diag/pg/share with a JSON body {to, marker}: in a shared database, the
// app makes diag_share_existing in its own schema and stores the marker, grants the role `to`
// read access exactly as SKILL.md §5 tells an agent to, then makes diag_share_new and stores
// the marker there too, so a reader proves both the grant on existing tables and the default
// privileges on later ones. Answers {schema} or {error}.
func diagShare(w http.ResponseWriter, req *http.Request) {
	var in struct {
		To     string `json:"to"`
		Marker string `json:"marker"`
	}
	if err := json.NewDecoder(req.Body).Decode(&in); err != nil || !shareRole.MatchString(in.To) || in.Marker == "" {
		writeJSON(w, 400, map[string]string{"error": "a JSON body with to (an app role) and marker is required"})
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()
	conn, err := diagConn(ctx)
	if err != nil {
		writeJSON(w, 200, map[string]string{"error": err.Error()})
		return
	}
	defer conn.Close(ctx)
	var schema string
	if err := conn.QueryRow(ctx, "select current_user").Scan(&schema); err != nil {
		writeJSON(w, 200, map[string]string{"error": err.Error()})
		return
	}
	s, to := pgx.Identifier{schema}.Sanitize(), pgx.Identifier{in.To}.Sanitize()
	steps := []struct {
		sql  string
		args []any
	}{
		{"create table if not exists " + s + ".diag_share_existing (marker text not null)", nil},
		{"insert into " + s + ".diag_share_existing (marker) values ($1)", []any{in.Marker}},
		{"grant usage on schema " + s + " to " + to, nil},
		{"grant select on all tables in schema " + s + " to " + to, nil},
		{"alter default privileges in schema " + s + " grant select on tables to " + to, nil},
		{"create table if not exists " + s + ".diag_share_new (marker text not null)", nil},
		{"insert into " + s + ".diag_share_new (marker) values ($1)", []any{in.Marker}},
	}
	for _, st := range steps {
		if _, err := conn.Exec(ctx, st.sql, st.args...); err != nil {
			writeJSON(w, 200, map[string]string{"schema": schema, "error": err.Error()})
			return
		}
	}
	writeJSON(w, 200, map[string]string{"schema": schema})
}

// diagRead is GET /diag/pg/read?schema=<another app's schema>&marker=<m>: whether this app
// can read the marker from that schema's two diag_share tables, and whether it can write
// there, which a read grant must not allow. Answers {tables: {name: {found, error}}, wrote,
// write_error}.
func diagRead(w http.ResponseWriter, req *http.Request) {
	schema, marker := req.URL.Query().Get("schema"), req.URL.Query().Get("marker")
	if !shareRole.MatchString(schema) || marker == "" {
		writeJSON(w, 400, map[string]string{"error": "schema (an app role) and marker are required"})
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()
	conn, err := diagConn(ctx)
	if err != nil {
		writeJSON(w, 200, map[string]string{"error": err.Error()})
		return
	}
	defer conn.Close(ctx)
	s := pgx.Identifier{schema}.Sanitize()
	tables := map[string]map[string]any{}
	for _, table := range []string{"diag_share_existing", "diag_share_new"} {
		var found bool
		// nosemgrep: go.lang.security.injection.tainted-sql-string.tainted-sql-string -- the schema name is quoted by pgx.Identifier.Sanitize
		err := conn.QueryRow(ctx, "select exists (select 1 from "+s+"."+table+" where marker = $1)", marker).Scan(&found)
		tables[table] = map[string]any{"found": found, "error": errText(err)}
	}
	// nosemgrep: go.lang.security.injection.tainted-sql-string.tainted-sql-string -- the schema name is quoted by pgx.Identifier.Sanitize
	_, err = conn.Exec(ctx, "insert into "+s+".diag_share_existing (marker) values ($1)", marker+"-reader")
	writeJSON(w, 200, map[string]any{"tables": tables, "wrote": err == nil, "write_error": errText(err)})
}

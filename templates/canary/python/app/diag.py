"""The diagnostics the platform's canary drives to prove the container's boundaries
(HARNESS.md H18, H22, H37 and H42). Every route is private, answers JSON, and is safe to delete in your
own app. They are the same in the three templates."""

import json
import os
import re
import socket
import urllib.error
import urllib.request
from typing import Any
from urllib.parse import urlsplit, urlunsplit

import psycopg
from psycopg import sql

# The edge's internal listener, where app-to-app calls arrive (CADDY.md section 4.3).
INTERNAL_PORT = "8443"


def diag_report() -> dict[str, Any]:
    """The process user, whether / and /tmp take writes, the container's own addresses,
    whether the host's Docker socket is visible, and the database role and name."""
    role, database = db_identity(os.environ.get("DATABASE_URL", ""))
    return {
        "uid": os.getuid() if hasattr(os, "getuid") else None,
        "root_writable": os.access("/", os.W_OK),
        "tmp_writable": os.access("/tmp", os.W_OK),
        "addrs": own_addrs(),
        "docker_socket": os.path.exists("/var/run/docker.sock"),
        "db_role": role,
        "db_name": database,
    }


def own_addrs() -> list[str]:
    """The container's IPv4 addresses in CIDR form, loopback left out."""
    out: list[str] = []
    try:
        import fcntl  # noqa: PLC0415
        import struct  # noqa: PLC0415

        for _, name in socket.if_nameindex():
            if name == "lo":
                continue
            with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
                packed = struct.pack("256s", name[:15].encode())
                try:
                    ip = socket.inet_ntoa(fcntl.ioctl(s.fileno(), 0x8915, packed)[20:24])
                    mask = socket.inet_ntoa(fcntl.ioctl(s.fileno(), 0x891B, packed)[20:24])
                except OSError:
                    continue
            bits = bin(int.from_bytes(socket.inet_aton(mask), "big")).count("1")
            out.append(f"{ip}/{bits}")
    except (OSError, ImportError):
        pass
    return out


def db_identity(raw: str) -> tuple[str, str]:
    u = urlsplit(raw)
    return u.username or "", u.path.lstrip("/")


def diag_call(to: str, with_token: bool = True, via: str | None = None) -> dict[str, Any]:
    """One request to the named app's internal name with this app's service identity (or
    none): the status the edge answered and the caller the target app saw. Only the apps this
    app declares in `calls` resolve from inside the container; `via` names one of them, and
    the request is sent to its address with `to` as the host, which is how the edge's refusal
    of an undeclared target is observed."""
    target = f"http://{via or to}.internal.whisk:{INTERNAL_PORT}/whoami"
    headers = {"Accept": "application/json", "Host": f"{to}.internal.whisk:{INTERNAL_PORT}"}
    if with_token:
        headers["Authorization"] = "Bearer " + os.environ.get("WHISK_SERVICE_TOKEN", "")
    req = urllib.request.Request(target, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=5) as resp:
            status, body = resp.status, resp.read()
    except urllib.error.HTTPError as e:
        status, body = e.code, e.read()
    except (urllib.error.URLError, OSError) as e:
        return {"to": to, "url": target, "status": 0, "error": str(e)}
    try:
        echoed = json.loads(body or b"{}")
    except ValueError:
        echoed = {}
    if not isinstance(echoed, dict):
        echoed = {}
    error = echoed.get("error") if isinstance(echoed.get("error"), dict) else {}
    return {
        "to": to,
        "url": target,
        "status": status,
        "service_app": echoed.get("x-whisk-service-app", ""),
        "audience": echoed.get("x-whisk-audience", ""),
        "code": error.get("code", "") if error else "",
    }


def diag_pg(role: str | None, database: str | None) -> dict[str, Any]:
    """A connection to the app's own database endpoint as another role or to another database,
    which the platform must refuse."""
    u = urlsplit(os.environ.get("DATABASE_URL", ""))
    if not u.username:
        return {"error": "DATABASE_URL is not a URL"}
    role = role or u.username
    database = database or u.path.lstrip("/")
    host = u.hostname or ""
    if ":" in host:
        host = "[" + host + "]"
    netloc = f"{role}:{u.password or ''}@{host}" + (f":{u.port}" if u.port else "")
    url = urlunsplit((u.scheme, netloc, "/" + database, u.query, ""))
    try:
        with psycopg.connect(url, connect_timeout=5) as conn:
            conn.execute("select 1")
        return {"role": role, "database": database, "connected": True, "error": ""}
    except psycopg.Error as e:
        return {"role": role, "database": database, "connected": False, "error": str(e).strip()}


def diag_post(app_id: Any, path: Any, headers: Any = None, body: Any = "") -> dict[str, Any] | None:
    """POST /diag/call with a JSON body {app_id, path, headers, body}: one POST to the named
    app's internal name at path, carrying this app's service token, the given headers (a map of
    name to value, optional) and the body as sent. Returns {status, body, headers} with what
    came back, or {status: 0, error} when the call never completed; None when the input is
    invalid, which the route answers 400. The harness uses it to show that the delivery headers
    an app forges on an app-to-app call are stripped and that a handler refuses the call."""
    headers = headers or {}
    if not isinstance(app_id, str) or not app_id or not isinstance(path, str) or not path.startswith("/") or not isinstance(body, str) or not isinstance(headers, dict):
        return None
    sent = {**{str(k): str(v) for k, v in headers.items()}, "Authorization": "Bearer " + os.environ.get("WHISK_SERVICE_TOKEN", "")}
    req = urllib.request.Request(f"http://{app_id}.internal.whisk:{INTERNAL_PORT}{path}", data=body.encode(), headers=sent, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            status, raw, got = resp.status, resp.read(1 << 16), resp.headers
    except urllib.error.HTTPError as e:
        status, raw, got = e.code, e.read(1 << 16), e.headers
    except (urllib.error.URLError, OSError) as e:
        return {"status": 0, "error": str(e)}
    return {"status": status, "body": raw.decode("utf-8", "replace"), "headers": {k.lower(): v for k, v in got.items()}}


# The shape of an app's database role, and so of its schema in a shared database
# (CONTRACT.md section 8).
SHARE_ROLE = re.compile(r"^app_[a-z0-9_]{1,59}$")


def _diag_conn() -> psycopg.Connection:
    """One autocommit connection to the app's own database, with no prepared statements
    because DATABASE_URL is pooled in transaction mode."""
    return psycopg.connect(os.environ.get("DATABASE_URL", ""), connect_timeout=5, autocommit=True, prepare_threshold=None)


def diag_share(to: Any, marker: Any) -> dict[str, Any] | None:
    """POST /diag/pg/share with a JSON body {to, marker}: in a shared database, the app makes
    diag_share_existing in its own schema and stores the marker, grants the role `to` read
    access exactly as SKILL.md section 5 tells an agent to, then makes diag_share_new and stores
    the marker there too. Returns {schema} or {error}; None when the input is invalid."""
    if not isinstance(to, str) or not SHARE_ROLE.match(to) or not isinstance(marker, str) or not marker:
        return None
    schema = ""
    try:
        with _diag_conn() as conn:
            schema = conn.execute("select current_user").fetchone()[0]
            s, t = sql.Identifier(schema), sql.Identifier(to)
            steps = [
                (sql.SQL("create table if not exists {}.diag_share_existing (marker text not null)").format(s), None),
                (sql.SQL("insert into {}.diag_share_existing (marker) values (%s)").format(s), (marker,)),
                (sql.SQL("grant usage on schema {} to {}").format(s, t), None),
                (sql.SQL("grant select on all tables in schema {} to {}").format(s, t), None),
                (sql.SQL("alter default privileges in schema {} grant select on tables to {}").format(s, t), None),
                (sql.SQL("create table if not exists {}.diag_share_new (marker text not null)").format(s), None),
                (sql.SQL("insert into {}.diag_share_new (marker) values (%s)").format(s), (marker,)),
            ]
            for statement, params in steps:
                conn.execute(statement, params)
        return {"schema": schema}
    except psycopg.Error as e:
        return {"schema": schema, "error": str(e).strip()}


def diag_read(schema: str, marker: str) -> dict[str, Any] | None:
    """GET /diag/pg/read?schema=<another app's schema>&marker=<m>: whether this app can read the
    marker from that schema's two diag_share tables, and whether it can write there, which a
    read grant must not allow. None when the input is invalid."""
    if not SHARE_ROLE.match(schema or "") or not marker:
        return None
    try:
        conn = _diag_conn()
    except psycopg.Error as e:
        return {"error": str(e).strip()}
    with conn:
        s = sql.Identifier(schema)
        tables: dict[str, dict[str, Any]] = {}
        for table in ("diag_share_existing", "diag_share_new"):
            try:
                row = conn.execute(sql.SQL("select exists (select 1 from {}.{} where marker = %s)").format(s, sql.Identifier(table)), (marker,)).fetchone()
                tables[table] = {"found": bool(row[0]), "error": ""}
            except psycopg.Error as e:
                tables[table] = {"found": False, "error": str(e).strip()}
        try:
            conn.execute(sql.SQL("insert into {}.diag_share_existing (marker) values (%s)").format(s), (marker + "-reader",))
            wrote, write_error = True, ""
        except psycopg.Error as e:
            wrote, write_error = False, str(e).strip()
    return {"tables": tables, "wrote": wrote, "write_error": write_error}

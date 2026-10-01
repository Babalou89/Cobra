"""HTTP service: POST /check, GET /map?unit=, GET /health, POST /bypass. Resolve is CLI-only (no HTTP route)."""
import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

from . import mapgen
from . import rules as R
from .scan import reconcile

ALLOWED_HOSTS = ("127.0.0.1", "10.0.0.1")
MAX_BODY = 8 * 1024 * 1024


class Librarian:
    """Service object: root + db + config + notifier. One instance, shared by server/watcher/CLI."""

    def __init__(self, root, db, config=None, notifier=None):
        self.root = root
        self.db = db
        self.cfg = config or R.DEFAULT
        self.notifier = notifier

    def check(self, actor, op, path, content=None, dest=None):
        return R.check(self.root, actor, op, path, content, dest=dest, config=self.cfg, lock=self.db.open_ids)

    def startup(self, open_incidents=False):
        """Startup reconciliation (replaces the daily audit): scan, sync image, tamper -> incidents."""
        vs, new = reconcile(self.root, self.db, self.cfg, open_incidents=open_incidents)
        if self.notifier:
            for iid in new:
                self.notifier.notify(self.db.get_incident(iid))
        return vs, new

    def health(self):
        return {"ok": True, "open_incidents": self.db.open_ids(), "files": len(self.db.files())}

    def map(self, unit):
        return mapgen.build_map(self.root, unit, self.cfg)

    def log_bypass(self, actor, detail):
        self.db.log_bypass(actor, detail)
        if self.notifier:
            self.notifier.notify_text("LIBRARIAN_BYPASS used by %s: %s" % (actor, detail))


def make_server(lib, host="127.0.0.1", port=0, allowed_hosts=ALLOWED_HOSTS):
    if host not in allowed_hosts:
        raise ValueError("host %r not allowed; use one of %s" % (host, ", ".join(allowed_hosts)))

    class H(BaseHTTPRequestHandler):
        server_version = "librarian/1"

        def log_message(self, *a):
            pass

        def _send(self, code, obj, text=False):
            body = (obj if text else json.dumps(obj)).encode()
            self.send_response(code)
            self.send_header("Content-Type", "text/plain; charset=utf-8" if text else "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def _body(self):
            n = int(self.headers.get("Content-Length") or 0)
            if n > MAX_BODY:
                raise ValueError("body too large")
            return json.loads(self.rfile.read(n) or b"{}")

        def do_GET(self):
            u = urlparse(self.path)
            if u.path == "/health":
                return self._send(200, lib.health())
            if u.path == "/map":
                unit = (parse_qs(u.query).get("unit") or [""])[0].strip("/")
                parts = unit.split("/")
                import os
                if len(parts) != 2 or not all(parts) or ".." in parts or not os.path.isdir(os.path.join(lib.root, *parts)):
                    return self._send(404, {"error": "unit must be <group>/<name> and exist"})
                return self._send(200, lib.map(unit), text=True)
            self._send(404, {"error": "not found"})

        def do_POST(self):
            u = urlparse(self.path)
            if u.path not in ("/check", "/bypass"):
                return self._send(404, {"error": "not found (resolve is CLI-only)"})
            try:
                req = self._body()
                if not isinstance(req, dict):
                    raise ValueError("body must be an object")
            except (ValueError, json.JSONDecodeError) as e:
                return self._send(400, {"error": "bad request: %s" % e})
            if u.path == "/bypass":
                lib.log_bypass(str(req.get("actor", "?")), str(req.get("detail", ""))[:2000])
                return self._send(200, {"logged": True})
            for k in ("actor", "op", "path"):
                if not isinstance(req.get(k), str) or not req.get(k):
                    return self._send(400, {"error": "missing field %s" % k})
            v = lib.check(req["actor"], req["op"], req["path"], req.get("content"), req.get("dest"))
            self._send(200, {"allow": v.allow, "rule_id": v.rule_id, "reason": v.reason, "suggested_name": v.suggested_name})

    srv = ThreadingHTTPServer((host, port), H)
    srv.daemon_threads = True
    return srv


def serve_in_thread(srv):
    t = threading.Thread(target=srv.serve_forever, daemon=True)
    t.start()
    return t

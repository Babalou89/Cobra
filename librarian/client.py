"""Client shim for hooks/CLI: exit 0 allow, 2 deny, 2 unreachable (FAIL CLOSED) unless LIBRARIAN_BYPASS=1 (logged)."""
import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request


def _post(url, path, obj, timeout):
    req = urllib.request.Request(url.rstrip("/") + path, data=json.dumps(obj).encode(),
                                 headers={"Content-Type": "application/json"}, method="POST")
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return json.loads(r.read())


def spool_path():
    return os.environ.get("LIBRARIAN_BYPASS_LOG") or os.path.expanduser("~/.librarian-bypass.log")


def _flush_spool(url, timeout):
    sp = spool_path()
    try:
        with open(sp) as f:
            lines = [l for l in f.read().splitlines() if l.strip()]
    except OSError:
        return
    left = []
    for l in lines:
        try:
            _post(url, "/bypass", json.loads(l), timeout)
        except Exception:
            left.append(l)
    try:
        if left:
            with open(sp, "w") as f:
                f.write("\n".join(left) + "\n")
        else:
            os.remove(sp)
    except OSError:
        pass


def run(url, actor, op, path, content=None, dest=None, timeout=3.0, env=None, err=sys.stderr):
    env = os.environ if env is None else env
    if env.get("LIBRARIAN_BYPASS") == "1":
        rec = {"actor": actor, "detail": "bypass %s %s%s at %s" % (op, path, " -> " + dest if dest else "", time.strftime("%FT%T"))}
        try:
            _post(url, "/bypass", rec, timeout)
        except Exception:
            try:
                with open(spool_path(), "a") as f:
                    f.write(json.dumps(rec) + "\n")
            except OSError:
                pass
        err.write("librarian: BYPASS (logged)\n")
        return 0
    try:
        body = {"actor": actor, "op": op, "path": path}
        if content is not None:
            body["content"] = content
        if dest:
            body["dest"] = dest
        r = _post(url, "/check", body, timeout)
    except (urllib.error.URLError, OSError, ValueError) as e:
        err.write("librarian unreachable (%s): failing closed. Set LIBRARIAN_BYPASS=1 only if Billy said so.\n" % e)
        return 2
    try:
        _flush_spool(url, timeout)
    except Exception:
        pass
    if r.get("allow"):
        return 0
    msg = "librarian DENY %s: %s" % (r.get("rule_id"), r.get("reason"))
    if r.get("suggested_name"):
        msg += " (suggested name: %s)" % r["suggested_name"]
    err.write(msg + "\n")
    return 2


def main(argv=None):
    ap = argparse.ArgumentParser(prog="librarian-client")
    ap.add_argument("--url", default=os.environ.get("LIBRARIAN_URL", "http://127.0.0.1:8765"))
    ap.add_argument("--content-file", help="file with the intended content ('-' = stdin)")
    ap.add_argument("--dest")
    ap.add_argument("--timeout", type=float, default=3.0)
    ap.add_argument("actor")
    ap.add_argument("op")
    ap.add_argument("path")
    a = ap.parse_args(argv)
    content = None
    if a.content_file:
        content = sys.stdin.read() if a.content_file == "-" else open(a.content_file, errors="replace").read()
    return run(a.url, a.actor, a.op, a.path, content, a.dest, a.timeout)


if __name__ == "__main__":
    sys.exit(main())

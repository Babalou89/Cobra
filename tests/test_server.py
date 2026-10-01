import json
import os
import urllib.error
import urllib.request
import unittest

from librarian.db import DB
from librarian.server import Librarian, make_server, serve_in_thread
from tests.helpers import TreeCase, hdr

U = "projects/demo/"


class ServerCase(TreeCase):
    def setUp(self):
        super().setUp()
        import shutil
        self.build()
        self.sdir = self.root + "-state"
        self.addCleanup(shutil.rmtree, self.sdir, True)
        self.db = DB(os.path.join(self.sdir, "lib.db"))
        self.addCleanup(self.db.close)
        self.lib = Librarian(self.root, self.db)
        self.srv = make_server(self.lib, "127.0.0.1", 0)
        serve_in_thread(self.srv)
        self.addCleanup(self.srv.server_close)
        self.addCleanup(self.srv.shutdown)
        self.url = "http://127.0.0.1:%d" % self.srv.server_address[1]

    def post(self, path, obj, raw=None):
        data = raw if raw is not None else json.dumps(obj).encode()
        req = urllib.request.Request(self.url + path, data=data, method="POST", headers={"Content-Type": "application/json"})
        try:
            with urllib.request.urlopen(req, timeout=5) as r:
                return r.status, json.loads(r.read())
        except urllib.error.HTTPError as e:
            return e.code, json.loads(e.read())

    def get(self, path):
        try:
            with urllib.request.urlopen(self.url + path, timeout=5) as r:
                return r.status, r.read().decode()
        except urllib.error.HTTPError as e:
            return e.code, e.read().decode()


class TestServer(ServerCase):
    def test_check_allow_and_deny(self):
        c, r = self.post("/check", {"actor": "a", "op": "create", "path": U + "access/adb/0002vadb.md", "content": hdr("adb", 2)})
        self.assertEqual((c, r["allow"], r["rule_id"]), (200, True, "OK"))
        c, r = self.post("/check", {"actor": "a", "op": "create", "path": U + "access/adb/notes.md"})
        self.assertEqual((c, r["allow"], r["rule_id"], r["suggested_name"]), (200, False, "R01", "0002vadb.md"))
        self.assertEqual(set(r), {"allow", "rule_id", "reason", "suggested_name"})

    def test_move_with_dest(self):
        c, r = self.post("/check", {"actor": "librarian", "op": "move", "path": U + "access/adb/0001vadb.md",
                                    "dest": U + "raw/legacy-2026-09-30/0001vadb.md"})
        self.assertTrue(r["allow"], r)

    def test_lock_through_http(self):
        self.db.open_incident("x", "create", "p", "R01", "d")
        c, r = self.post("/check", {"actor": "a", "op": "create", "path": U + "access/adb/0002vadb.md"})
        self.assertEqual((r["allow"], r["rule_id"]), (False, "LOCK"))
        c, h = self.get("/health")
        self.assertEqual(json.loads(h)["open_incidents"], [1])

    def test_bad_requests(self):
        self.assertEqual(self.post("/check", None, raw=b"not json")[0], 400)
        self.assertEqual(self.post("/check", {"actor": "a"})[0], 400)
        self.assertEqual(self.post("/check", [1, 2])[0], 400)

    def test_resolve_not_over_http(self):
        self.assertEqual(self.post("/resolve", {"id": 1, "resolution": "approve"})[0], 404)

    def test_health_and_map(self):
        c, h = self.get("/health")
        self.assertEqual(c, 200)
        self.assertTrue(json.loads(h)["ok"])
        c, m = self.get("/map?unit=projects/demo")
        self.assertEqual(c, 200)
        self.assertIn("- access/adb/ | v0001 | open | s", m)
        self.assertEqual(self.get("/map?unit=projects/nope")[0], 404)
        self.assertEqual(self.get("/map?unit=../etc")[0], 404)
        self.assertEqual(self.get("/map")[0], 404)
        self.assertEqual(self.get("/nothing")[0], 404)

    def test_bypass_endpoint(self):
        self.post("/bypass", {"actor": "billy", "detail": "emergency"})
        self.assertEqual(self.db.bypasses()[0]["actor"], "billy")


class TestHosts(unittest.TestCase):
    def test_host_restriction(self):
        lib = Librarian("/tmp", DB(":memory:"))
        for h in ("0.0.0.0", "192.168.50.20", "localhost", ""):
            with self.assertRaises(ValueError):
                make_server(lib, h, 0)


if __name__ == "__main__":
    unittest.main()

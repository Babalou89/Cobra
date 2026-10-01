import os
import tempfile
import unittest

from librarian.notify import LogNotifier, MultiNotifier, NtfyNotifier, format_incident

INC = {"id": 7, "actor": "out-of-band", "path": "projects/demo/fyt.prop", "rule": "R05", "detail": "file not allowed"}


class TestNotify(unittest.TestCase):
    def test_format_has_required_fields(self):
        m = format_incident(INC)
        for needle in ("#7", "out-of-band", "projects/demo/fyt.prop", "R05", "librarian resolve 7 approve|revert"):
            self.assertIn(needle, m)

    def test_log_notifier_records_and_writes(self):
        d = tempfile.mkdtemp()
        n = LogNotifier(os.path.join(d, "sub", "alarm.log"))
        n.notify(INC)
        self.assertEqual(len(n.messages), 1)
        with open(os.path.join(d, "sub", "alarm.log")) as f:
            self.assertIn("#7", f.read())

    def test_multi(self):
        a, b = LogNotifier(), LogNotifier()
        MultiNotifier(a, b).notify_text("x")
        self.assertEqual((len(a.messages), len(b.messages)), (1, 1))

    def test_ntfy_request_built_not_sent(self):
        n = NtfyNotifier("http://10.0.0.1:2586/", "librarian")
        r = n.build_request("hello")
        self.assertEqual(r.full_url, "http://10.0.0.1:2586/librarian")
        self.assertEqual(r.data, b"hello")
        self.assertEqual(r.get_method(), "POST")


if __name__ == "__main__":
    unittest.main()

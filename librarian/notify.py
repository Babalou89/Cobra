"""Notifiers. Tests use LogNotifier only; NtfyNotifier.send() is the only network code and is never called in tests."""
import os
import time
import urllib.request


def format_incident(inc):
    return ("LIBRARIAN INCIDENT #%s | actor=%s | path=%s | rule=%s | %s | resolve: librarian resolve %s approve|revert"
            % (inc["id"], inc["actor"], inc["path"], inc["rule"], inc["detail"], inc["id"]))


class Notifier:
    def notify(self, incident):
        self.notify_text(format_incident(incident))

    def notify_text(self, text):
        raise NotImplementedError


class LogNotifier(Notifier):
    def __init__(self, path=None):
        self.path = path
        self.messages = []

    def notify_text(self, text):
        self.messages.append((time.time(), text))
        if self.path:
            os.makedirs(os.path.dirname(os.path.abspath(self.path)), exist_ok=True)
            with open(self.path, "a") as f:
                f.write("%s %s\n" % (time.strftime("%FT%T"), text))


class NtfyNotifier(Notifier):
    """Self-hosted ntfy: POST <base_url>/<topic> with the message as the body."""

    def __init__(self, base_url, topic, timeout=5, priority="urgent"):
        self.base_url, self.topic, self.timeout, self.priority = base_url.rstrip("/"), topic, timeout, priority

    def build_request(self, text):
        return urllib.request.Request("%s/%s" % (self.base_url, self.topic), data=text.encode(), method="POST",
                                      headers={"Title": "librarian alarm", "Priority": self.priority, "Tags": "rotating_light"})

    def notify_text(self, text):
        try:
            urllib.request.urlopen(self.build_request(text), timeout=self.timeout).read()
        except Exception:
            pass  # alarm failure must never break enforcement; LogNotifier in a MultiNotifier is the backstop


class MultiNotifier(Notifier):
    def __init__(self, *notifiers):
        self.notifiers = notifiers

    def notify_text(self, text):
        for n in self.notifiers:
            n.notify_text(text)

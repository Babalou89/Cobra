"""fyt-7862 target tree from docs/0029v section 4 (ordered: dirs/specs first)."""
from tests.helpers import hdr, SPEC_OK

U = "projects/fyt-7862/"

SPEC = "# fyt-7862\n\nGoal: diagnose and root the FYT 7862 head unit.\n\n## Map\n\nCurrent: hardware/0001vhardware.md\nNext: none\n"

# (op, relpath, content)
FIXTURE = [
    ("create", U + "SPEC.md", SPEC),
    ("create", U + "MEMORY.md", "2026-09-30 migrated\n"),
    ("create", U + "hardware/0001vhardware.md", hdr("hardware", 1, summary="build fingerprint, MCU, panel, android version")),
    ("create", U + "access/adb/0001vadb.md", hdr("adb", 1)),
    ("create", U + "access/serial/0001vserial.md", hdr("serial", 1)),
    ("create", U + "access/usb/0001vusb.md", hdr("usb", 1)),
    ("create", U + "rooting/attempts/0001vattempts.md", hdr("attempts", 1, summary="every attempt, Outcome: failed|worked")),
    ("create", U + "rooting/troubleshooting/0001vtroubleshooting.md", hdr("troubleshooting", 1, status="blocked")),
    ("create", U + "rooting/success/", None),
    ("create", U + "config/0001vconfig.md", hdr("config", 1)),
    ("create", U + "scripts/diagnostic/0001vdiagnostic.py", "print('diag')\n"),
    ("create", U + "scripts/adb-connect/0001vadb-connect.sh", "#!/bin/sh\n"),
    ("create", U + "scripts/root-adb-fix/0001vroot-adb-fix.sh", "#!/bin/sh\n"),
    ("create", U + "references/0001vreferences.md", hdr("references", 1)),
    ("create", U + "raw/firmware/joying_firmware.zip", "zip"),
    ("create", U + "raw/firmware/FYTuis7862BinRepo/README.md", "vendor"),
    ("create", U + "raw/dumps/diagnostic_20260923_020426/adb_devices.txt", "x"),
    ("create", U + "raw/dumps/0004Vmybtinfo.bin", "bin"),
    ("create", U + "raw/legacy-2026-09-30/0000Vconfig.txt", "legacy"),
    ("create", U + "raw/legacy-2026-09-30/0013Vsurfer63_root_links.md", "legacy"),
    ("create", U + "raw/legacy-2026-09-30/STATUS_REPORT_FINAL.txt", "legacy"),
    ("create", U + "raw/legacy-2026-09-30/FINAL_REPORT_20260923.txt", "legacy"),
    ("create", U + "raw/fyt.prop", "ro.x=1"),
    # follow-on versions
    ("create", U + "rooting/attempts/0002vattempts.md", hdr("attempts", 2)),
    ("write", U + "SPEC.md", SPEC.replace("none", "next step")),
]

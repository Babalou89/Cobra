#!/usr/bin/env python3
"""
Codebase graph refresh + change detection.
Run nightly via cron or after significant commits.

Indexes cobra and babaface, detects changes since last run,
saves reports to /home/billy/reports/.
"""

import json
import os
import subprocess
import sys
from datetime import datetime
from pathlib import Path

REPORTS_DIR = Path("/home/billy/reports")
REPORTS_DIR.mkdir(exist_ok=True)

PROJECTS = [
    {"name": "cobra", "path": "/home/billy/cobra", "branch": "main"},
    {"name": "babaface", "path": "/home/billy/babaface", "branch": "main"},
]

def run_cmd(cmd, cwd=None):
    """Run a command and return stdout."""
    try:
        result = subprocess.run(cmd, shell=True, capture_output=True, text=True, cwd=cwd, timeout=300)
        return result.stdout.strip(), result.returncode
    except subprocess.TimeoutExpired:
        return "TIMEOUT", 1

def get_git_ref(path):
    """Get current HEAD ref."""
    out, _ = run_cmd("git rev-parse HEAD", cwd=path)
    return out

def get_git_log_since(path, since_ref):
    """Get git log since a ref."""
    out, _ = run_cmd(f"git log --oneline {since_ref}..HEAD", cwd=path)
    return out

def index_project(name, path):
    """Index a project via the MCP tool (called externally)."""
    # This is a placeholder — the actual indexing happens via the MCP tool
    # in the hermes session. This script just orchestrates.
    return True

def main():
    timestamp = datetime.now().strftime("%Y-%m-%d_%H%M%S")
    report = {"timestamp": timestamp, "projects": []}
    
    for proj in PROJECTS:
        name = proj["name"]
        path = proj["path"]
        
        if not os.path.exists(path):
            report["projects"].append({"name": name, "status": "NOT_FOUND"})
            continue
        
        # Get current ref
        current_ref = get_git_ref(path)
        
        # Check for last known ref
        ref_file = REPORTS_DIR / f"{name}_last_ref.txt"
        last_ref = ref_file.read_text().strip() if ref_file.exists() else None
        
        # Get changes since last ref
        changes = ""
        if last_ref and last_ref != current_ref:
            changes = get_git_log_since(path, last_ref)
        
        # Save current ref
        ref_file.write_text(current_ref)
        
        proj_report = {
            "name": name,
            "current_ref": current_ref[:12],
            "last_ref": last_ref[:12] if last_ref else "FIRST_RUN",
            "changes": changes.split("\n") if changes else [],
            "status": "INDEXED" if not changes else "CHANGED"
        }
        report["projects"].append(proj_report)
    
    # Save report
    report_file = REPORTS_DIR / f"codebase_report_{timestamp}.json"
    with open(report_file, "w") as f:
        json.dump(report, f, indent=2)
    
    # Also save latest
    latest_file = REPORTS_DIR / "latest_report.json"
    with open(latest_file, "w") as f:
        json.dump(report, f, indent=2)
    
    # Print summary
    for proj in report["projects"]:
        status = proj["status"]
        name = proj["name"]
        changes = len(proj.get("changes", []))
        print(f"  {name}: {status} ({changes} commits since last check)")
    
    return 0

if __name__ == "__main__":
    sys.exit(main())

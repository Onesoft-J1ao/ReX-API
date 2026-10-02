"""Report duplicate top-level var declarations and duplicate switch cases.

Run after a multi-fork merge: parallel forks often add the same constant or the
same case branch to the same file, and git merges those silently because the
lines differ enough textually.
"""
import collections
import re
import subprocess
import sys

files = subprocess.check_output(
    ["git", "diff", "--name-only", "--diff-filter=U"], text=True).split()
if not files:
    files = subprocess.check_output(
        ["git", "diff", "--name-only", "HEAD"], text=True).split()

problems = 0
for path in files:
    if not path.endswith(".go"):
        continue
    try:
        with open(path, encoding="utf-8") as fh:
            lines = fh.read().split("\n")
    except OSError:
        continue

    # --- duplicate top-level var / const names
    seen = collections.defaultdict(list)
    for i, line in enumerate(lines):
        m = re.match(r"^(?:var|const)\s+([A-Za-z_]\w*)\s*=", line)
        if m:
            seen[m.group(1)].append(i)
    for name, idxs in sorted(seen.items()):
        if len(idxs) > 1:
            problems += 1
            print("  DUP VAR  %s:%s  %s"
                  % (path, [i + 1 for i in idxs], name))

    # --- duplicate case labels inside each function body
    depth = 0
    cases = collections.defaultdict(list)
    for i, line in enumerate(lines):
        stripped = line.strip()
        m = re.match(r"""^case\s+(.*?):\s*$""", stripped)
        if m and depth > 0:
            for label in m.group(1).split(","):
                label = label.strip()
                if label:
                    cases[label].append(i)
        depth += line.count("{") - line.count("}")
    for label, idxs in sorted(cases.items()):
        if len(set(idxs)) > 1 and not label.startswith("//"):
            # only flag string-literal cases, which are the merge-prone kind
            if label.startswith('"'):
                problems += 1
                print("  DUP CASE %s:%s  %s"
                      % (path, [i + 1 for i in idxs], label))

print("  total: %d problem(s)" % problems)
sys.exit(0)
#!/usr/bin/env python3
"""Regenerate the CI tool coverage matrix in docs/reference/language-support.md
from a CI run's own results.

TestMultiLanguage prints a summary table per job (one row per language, one
column per tool, ✓ pass / - skip / ✗ fail). This script reads those tables from
a CI log and rewrites the matrix rows, so the matrix shows what CI observed
rather than what was typed by hand.

Usage:
    gh run view <run-id> --log > ci.log
    python3 scripts/update-coverage-matrix.py ci.log

A language with no summary row in the log (it did not run end to end) keeps
its current row; check those by hand.
"""
import re
import sys

DOC = "docs/reference/language-support.md"
ICON = {"✓": "pass", "-": "—", "✗": "fail"}
DOC_TO_CI_COLUMN = {"Tier 1": "T1"}
DOC_TO_CI_LANGUAGE = {"C#": "CSharp"}


def parse_ci_rows(log_text):
    cols, rows = None, {}
    for line in log_text.split("\n"):
        if "| T1 |" in line and "Language" in line:
            cols = [c.strip() for c in line[line.index("Language"):].split("|")]
            continue
        if not cols or " | " not in line:
            continue
        body = line.split("Z ", 1)[-1].strip() if "Z " in line else line
        parts = [p.strip() for p in body.split("|")]
        if len(parts) == len(cols) and parts[1] in ICON:
            name = re.sub(r"^.*?\.go:\d+:\s*", "", parts[0])
            rows[name] = dict(zip(cols, parts))
    return rows


def main():
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    rows = parse_ci_rows(open(sys.argv[1], encoding="utf-8", errors="replace").read())
    if not rows:
        sys.exit("no TestMultiLanguage summary rows found in the log")

    lines = open(DOC, encoding="utf-8").read().split("\n")
    hdr_i = next(i for i, l in enumerate(lines) if l.startswith("| Language | Tier 1 |"))
    header = [c.strip() for c in lines[hdr_i].strip("|").split("|")]
    updated, not_run = 0, []
    i = hdr_i + 2
    while i < len(lines) and lines[i].startswith("|"):
        cells = [c.strip() for c in lines[i].strip("|").split("|")]
        lang = cells[0]
        row = rows.get(DOC_TO_CI_LANGUAGE.get(lang, lang))
        if row is None:
            not_run.append(lang)
        else:
            values = [ICON[row[DOC_TO_CI_COLUMN.get(h, h)]] for h in header[1:]]
            lines[i] = "| " + " | ".join([lang] + values) + " |"
            updated += 1
        i += 1
    open(DOC, "w", encoding="utf-8").write("\n".join(lines))
    print(f"updated {updated} rows in {DOC}")
    if not_run:
        print("no CI summary row (left unchanged): " + ", ".join(not_run))


if __name__ == "__main__":
    main()

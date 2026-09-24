"""retired-rules: the retired-rule records in schema/registry.json.

The registry field is named `tombstones`; each record says when the rule was
retired, why, what replaced it, and what a consumer should do.

Usage: :-: retired-rules
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

import _common as c


def resolve(attrs, config, body):
    records = c.load_registry()["tombstones"]
    if not records:
        return "No rule has been retired."
    rows = []
    for record in records:
        successors = ", ".join(c.rule_link(r) for r in record["replaced_by"]) or "none"
        rows.append([c.code(record["id"]), record["retired_in"], record["reason"], successors, record["migration"]])
    return c.table(["Rule", "Retired in", "Why", "Replaced by", "What to do"], rows)

"""rule-table: every live rule from schema/registry.json, one row each.

Usage: :-: rule-table
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

import _common as c


def resolve(attrs, config, body):
    registry = c.load_registry()
    rows = []
    for rule in registry["rules"]:
        groups = ", ".join(f"`group:{g}`" for g in rule["groups"]) or "none"
        rows.append([
            c.rule_link(rule["id"]),
            rule["severity"],
            rule["description"],
            c.code(rule["suppression"]),
            f"tier {rule['fix_tier']}",
            groups,
        ])
    return c.table(["Rule", "Default severity", "Detects", "Suppression shape", "Fix", "Groups"], rows)

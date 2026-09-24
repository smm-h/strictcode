"""group-table: every rule group and its members, from schema/registry.json.

Usage: :-: group-table
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

import _common as c


def resolve(attrs, config, body):
    groups = c.load_registry()["groups"]
    if not groups:
        return "No groups are declared."
    rows = [[f"`group:{name}`", ", ".join(c.rule_link(member) for member in members)] for name, members in sorted(groups.items())]
    return c.table(["Group", "Members"], rows)

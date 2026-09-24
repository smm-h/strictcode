"""support-matrix: rules by language, from the support cells in schema/registry.json.

The cells are computed by the Go matrix calculus and written by
`strictcode registry dump`; this directive only lays them out.

Usage: :-: support-matrix
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

import _common as c


def resolve(attrs, config, body):
    registry = c.load_registry()
    languages = registry["languages"]
    headers = ["Rule"] + [lang["display_name"] for lang in languages]
    rows = []
    independent = []
    for rule in registry["rules"]:
        if rule["language_independent"]:
            independent.append(rule)
            continue
        rows.append([c.rule_link(rule["id"])] + [c.support_text(rule["support"][lang["id"]]) for lang in languages])
    out = [c.table(headers, rows)]
    if independent:
        out.append("")
        out.append("Rules that engage no language capability apply to every project regardless of language:")
        out.append("")
        for rule in independent:
            out.append(f"- {c.rule_link(rule['id'])}: {rule['description']}")
    return "\n".join(out)

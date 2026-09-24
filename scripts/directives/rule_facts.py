"""rule-facts: the registry facts for one rule, as a table plus per-language support.

Usage: :-: rule-facts id="deps-unused"
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

import _common as c


def resolve(attrs, config, body):
    rule_id = c.require(attrs, "id", "rule-facts")
    registry = c.load_registry()
    matches = [rule for rule in registry["rules"] if rule["id"] == rule_id]
    if not matches:
        retired = [t for t in registry["tombstones"] if t["id"] == rule_id]
        if retired:
            raise ValueError(f"rule-facts: {rule_id} is retired; document it with retired-rules instead")
        raise ValueError(f"rule-facts: no rule {rule_id} in {c.REGISTRY}")
    rule = matches[0]

    def caps(names):
        return ", ".join(c.code(name) for name in names) or "none"

    planned = "; ".join(f"tier {p['tier']}: {p['description']}" for p in rule["planned_fixes"]) or "none"
    facts = c.table(["Fact", "Value"], [
        ["Detects", rule["description"]],
        ["Default severity", rule["severity"]],
        ["Requires capabilities", caps(rule["requires"])],
        ["Uses capabilities (optional enrichment)", caps(rule["uses"])],
        ["Groups", ", ".join(f"`group:{g}`" for g in rule["groups"]) or "none"],
        ["Suppression shape", c.code(rule["suppression"])],
        ["Fix offered", f"tier {rule['fix_tier']}"],
        ["Planned fixes", planned],
    ])

    if rule["language_independent"]:
        support = "This rule engages no language capability, so it applies to every project regardless of language."
    else:
        names = {lang["id"]: lang["display_name"] for lang in registry["languages"]}
        rows = [[names[lang["id"]], c.support_text(rule["support"][lang["id"]])] for lang in registry["languages"]]
        support = c.table(["Language", "Support"], rows)
    return facts + "\n\n" + support

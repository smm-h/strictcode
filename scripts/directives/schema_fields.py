"""schema-fields: the fields of one record type in a strictspec schema, as a table.

Reads the schema file itself, so a page documenting strictcode.toml or the
findings document cannot drift from what the validator enforces.

Usage: :-: schema-fields path="schema/strictspec/config.schema.toml" type="Suppression"
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

import _common as c


def describe_type(site):
    kind = site["type"]
    if kind == "enum":
        if "values" in site:
            return "one of " + ", ".join(c.code(v) for v in site["values"])
        source = site["source"]
        return f"one of the values of `{source['selector']}` in `{source['document']}`"
    if kind == "array":
        return "array of " + describe_type(site["item"])
    if kind == "map":
        return f"map (keys matching `{site['key_pattern']}`) of " + describe_type(site["value"])
    return c.code(kind)


def resolve(attrs, config, body):
    path = c.require(attrs, "path", "schema-fields")
    type_name = c.require(attrs, "type", "schema-fields")
    schema = c.load_toml(path)
    types = schema.get("types", {})
    if type_name not in types:
        raise ValueError(f"schema-fields: {path} declares no type {type_name!r}")
    record = types[type_name]
    if record.get("type") != "record":
        raise ValueError(f"schema-fields: {type_name} in {path} is a {record.get('type')}, not a record")
    rows = []
    for name, site in record.get("fields", {}).items():
        rows.append([
            c.code(name),
            describe_type(site),
            "yes" if site.get("required") else "no",
            site.get("description", ""),
        ])
    out = c.table(["Field", "Type", "Required", "Description"], rows)
    constraints = record.get("constraints", [])
    if constraints:
        lines = ["", "Constraints the validator enforces on this record:", ""]
        for constraint in constraints:
            lines.append("- " + describe_constraint(constraint))
        out += "\n" + "\n".join(lines)
    return out


def describe_constraint(constraint):
    form = constraint["form"]
    if "fields" in constraint:
        return f"{form}: " + ", ".join(c.code(f) for f in constraint["fields"])
    if "when" in constraint:
        when = constraint["when"]
        target = when.get("value", when.get("values"))
        return f"{form}: {c.code(constraint['field'])} when {c.code(when['field'])} {when['predicate']} {c.code(target)}"
    if form == "unique-by":
        return f"unique-by: {c.code(constraint['field'])} within {c.code(constraint['collection'])}"
    return form

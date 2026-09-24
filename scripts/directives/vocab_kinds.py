"""vocab-kinds: node kinds or row kinds from schema/vocabulary.toml.

Usage:
  :-: vocab-kinds kind="node"
  :-: vocab-kinds kind="row"
  :-: vocab-kinds kind="layer"
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

import _common as c


def attributes(entry):
    attrs = entry.get("attributes", [])
    return ", ".join(f"`{a['name']}` ({a['type']})" for a in attrs) or "none"


def resolve(attrs, config, body):
    kind = c.require(attrs, "kind", "vocab-kinds")
    vocabulary = c.load_vocabulary()
    if kind == "node":
        rows = [[c.code(k["id"]), k["maturity"], k["description"], attributes(k)] for k in vocabulary["node_kinds"]]
        return c.table(["Node kind", "Maturity", "Description", "Attributes"], rows)
    if kind == "row":
        rows = [[
            c.code(k["id"]),
            k["maturity"],
            ", ".join(c.code(s) for s in k["src_kinds"]),
            ", ".join(c.code(d) for d in k["dst_kinds"]),
            k["description"],
            attributes(k),
        ] for k in vocabulary["row_kinds"]]
        return c.table(["Row kind", "Maturity", "From", "To", "Description", "Attributes"], rows)
    if kind == "layer":
        rows = []
        for layer in vocabulary["layers"]:
            members = [c.code(cap["id"]) for cap in vocabulary["capabilities"] if cap["layer"] == layer["id"]]
            rows.append([c.code(layer["id"]), layer["maturity"], layer["description"], ", ".join(members)])
        return c.table(["Layer", "Maturity", "Description", "Capabilities"], rows)
    raise ValueError(f'vocab-kinds: kind must be "node", "row", or "layer", not {kind!r}')

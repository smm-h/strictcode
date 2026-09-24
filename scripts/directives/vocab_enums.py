"""vocab-enums: the enumerations declared in schema/vocabulary.toml.

Usage:
  :-: vocab-enums
  :-: vocab-enums ids="provenance, discipline, mechanism"
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

import _common as c


def resolve(attrs, config, body):
    enums = {entry["id"]: entry["values"] for entry in c.load_vocabulary()["enums"]}
    wanted = attrs.get("ids")
    ids = [name.strip() for name in wanted.split(",")] if wanted else list(enums)
    unknown = [name for name in ids if name not in enums]
    if unknown:
        raise ValueError(f"vocab-enums: the vocabulary declares no enums named {unknown}")
    rows = [[c.code(name), ", ".join(c.code(v) for v in enums[name])] for name in ids]
    return c.table(["Enumeration", "Values"], rows)

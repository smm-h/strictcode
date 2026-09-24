"""capability-status: every vocabulary capability with its status in each language profile.

Usage:
  :-: capability-status
  :-: capability-status layer="import-graph"
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

import _common as c


def resolve(attrs, config, body):
    vocabulary = c.load_vocabulary()
    profiles = c.load_profiles()
    layer = attrs.get("layer")
    layers = {entry["id"] for entry in vocabulary["layers"]}
    if layer is not None and layer not in layers:
        raise ValueError(f"capability-status: unknown layer {layer!r}; the vocabulary declares {sorted(layers)}")

    headers = ["Capability"] + ([] if layer else ["Layer"]) + [p["display_name"] for p in profiles.values()]
    rows = []
    for cap in vocabulary["capabilities"]:
        if layer and cap["layer"] != layer:
            continue
        row = [c.code(cap["id"])] + ([] if layer else [cap["layer"]])
        for lang, profile in profiles.items():
            status = profile["capabilities"].get(cap["id"])
            if status is None:
                raise ValueError(f"capability-status: profile {lang} declares no status for {cap['id']}")
            if status["status"] == "not-applicable":
                row.append("n/a: " + status["reason"])
            else:
                row.append(status["status"])
        rows.append(row)
    return c.table(headers, rows)

"""profile-constructs: how one language's constructs map onto the vocabulary.

Usage: :-: profile-constructs lang="py"
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

import _common as c


def resolve(attrs, config, body):
    lang = c.require(attrs, "lang", "profile-constructs")
    profiles = c.load_profiles()
    if lang not in profiles:
        raise ValueError(f"profile-constructs: no profile for {lang!r}; profiles exist for {list(profiles)}")
    profile = profiles[lang]
    rows = []
    for construct in profile["constructs"]:
        if "node_kind" in construct:
            target = "node " + c.code(construct["node_kind"])
        else:
            target = "row " + c.code(construct["row_kind"])
        rows.append([construct["construct"], target, construct["notes"]])
    intro = f"Grammar: `{profile['grammar']}`."
    return intro + "\n\n" + c.table(["Construct", "Becomes", "Notes"], rows)

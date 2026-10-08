"""Shared helpers for strictcode's custom selfdoc directives.

Every directive renders Markdown from a committed machine-read artifact:
schema/registry.json (written by `strictcode registry dump`),
schema/vocabulary.toml, schema/profiles/*.toml, the strictspec schemas under
schema/strictspec/, and .strictmetadata/.cli-schema/schema.json. None of them recomputes
anything the Go code decides; each one only formats data. Every missing file,
unknown key, or malformed value raises, so selfdoc stops the build instead of
publishing a page with a hole in it.
"""

import json
import tomllib
from pathlib import Path

REGISTRY = Path("schema/registry.json")
VOCABULARY = Path("schema/vocabulary.toml")
PROFILES = Path("schema/profiles")
CLI_SCHEMA = Path(".strictmetadata/.cli-schema/schema.json")


def load_registry():
    with REGISTRY.open(encoding="utf-8") as handle:
        return json.load(handle)


def load_vocabulary():
    with VOCABULARY.open("rb") as handle:
        return tomllib.load(handle)


def load_profiles():
    """Return the profiles keyed by language id, in the registry's column order."""
    by_lang = {}
    for path in sorted(PROFILES.glob("*.toml")):
        with path.open("rb") as handle:
            doc = tomllib.load(handle)
        by_lang[doc["language"]] = doc
    order = [lang["id"] for lang in load_registry()["languages"]]
    missing = [lang for lang in order if lang not in by_lang]
    if missing:
        raise ValueError(f"no profile under {PROFILES} for languages {missing}")
    return {lang: by_lang[lang] for lang in order}


def load_toml(path):
    with Path(path).open("rb") as handle:
        return tomllib.load(handle)


def load_cli_schema():
    with CLI_SCHEMA.open(encoding="utf-8") as handle:
        return json.load(handle)


def require(attrs, name, directive):
    value = attrs.get(name)
    if not value:
        raise ValueError(f'{directive} needs {name}="..."')
    return value


def cell(text):
    """Escape a value for a Markdown table cell."""
    return str(text).replace("|", "\\|").replace("\n", " ")


def code(text):
    return f"`{text}`"


def table(headers, rows):
    out = ["| " + " | ".join(headers) + " |", "|" + "---|" * len(headers)]
    for row in rows:
        out.append("| " + " | ".join(cell(value) for value in row) + " |")
    return "\n".join(out)


def rule_link(rule_id):
    return f"[`{rule_id}`](../rules/{rule_id}/)"


def support_text(entry):
    if entry["status"] == "n/a":
        return "n/a: " + entry["reason"]
    return entry["status"]

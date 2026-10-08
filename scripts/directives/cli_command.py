"""cli-command: one strictcode command's arguments and flags, from .strictmetadata/.cli-schema/schema.json.

Usage:
  :-: cli-command name="analyze"
  :-: cli-command name="registry dump"
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

import _common as c


def find(schema, words):
    node = {"commands": schema["commands"], "groups": schema["groups"]}
    for i, word in enumerate(words):
        last = i == len(words) - 1
        if last and word in node.get("commands", {}):
            return node["commands"][word]
        if not last and word in node.get("groups", {}):
            node = node["groups"][word]
            continue
        raise ValueError(f"cli-command: {' '.join(words)!r} is not a command in {c.CLI_SCHEMA}")
    raise ValueError("cli-command: empty command name")


def presence(entry):
    if entry["presence"] == "default":
        return f"defaults to `{entry['default']}`"
    return entry["presence"]


def resolve(attrs, config, body):
    name = c.require(attrs, "name", "cli-command")
    schema = c.load_cli_schema()
    # The schema omits every value equal to its declared default, so merge the
    # defaults back in rather than assuming a key is present.
    command = {**schema["defaults"]["command"], **find(schema, name.split())}
    flag_defaults = schema["defaults"]["flag"]
    out = [f"`strictcode {name}`: {command['help']}.", ""]
    out.append(f"Effect: {command['effect']}.")
    if command["dry_run_supported"]:
        out.append("`--dry-run` is accepted.")
    else:
        out.append(f"`--dry-run` is refused: {command['dry_run_unsupported_reason']}.")
    if command["payload_schema"] is not None:
        out.append("Under `--json`, stdout carries one strictcli JSON document whose `payload` member is validated against the schema this command declares.")
    rows = []
    for arg in command["args"]:
        rows.append([c.code(arg["name"]), "argument", presence(arg), arg["help"]])
    for raw in command["flags"]:
        flag = {**flag_defaults, **raw}
        if flag["choices"]:
            members = ", ".join(f"`--{choice['name']}` ({choice['help']})" for choice in flag["choices"])
            rows.append([c.code(flag["name"]), "choose one flag", flag["presence"], f"{flag['help']}: {members}"])
        else:
            rows.append([c.code("--" + flag["name"]), "flag", presence(flag), flag["help"]])
    if rows:
        out += ["", c.table(["Name", "Kind", "Presence", "Meaning"], rows)]
    return "\n".join(out)

"""Render the production template with individual Actions variables and secrets."""

import json
import os
from pathlib import Path
import re
import sys


PLACEHOLDER = "\u5077apikey\u7684\u66ff\u6211\u6321\u707e"
SECRET_NAMES = {
    "MONGO_PASSWORD", "OSS_ACCESS_KEY_ID", "OSS_ACCESS_KEY_SECRET",
    "DASHSCOPE_API_KEY", "DEEPSEEK_API_KEY", "SMS_ACCESS_KEY_ID",
    "SMS_ACCESS_KEY_SECRET", "SMS_SECURITY_TOKEN",
}
VARIABLE_NAMES = {"MONGO_AUTH_SOURCE"}


def render(source, output, environ):
    """Resolve whole-value placeholders without shell or raw JSON substitution."""
    if not environ.get("MONGO_PASSWORD") or environ["MONGO_PASSWORD"] == PLACEHOLDER:
        raise ValueError("MONGO_PASSWORD is required on every deployment.")
    config = json.loads(Path(source).read_text(encoding="utf-8-sig"))

    def resolve(value):
        if isinstance(value, dict):
            return {key: resolve(item) for key, item in value.items()}
        if isinstance(value, list):
            return [resolve(item) for item in value]
        if isinstance(value, str) and "${" in value:
            if not re.fullmatch(r"\$\{[A-Z_]+\}", value):
                raise ValueError("Production placeholders must occupy a whole JSON value.")
            name = value[2:-1]
            if name in SECRET_NAMES:
                return environ.get(name) or ("" if name == "SMS_SECURITY_TOKEN" else PLACEHOLDER)
            if name not in VARIABLE_NAMES:
                raise ValueError("Unsupported production variable name.")
            return environ.get(name, "")
        return value

    config = resolve(config)
    path = Path(output)
    path.parent.mkdir(parents=True, exist_ok=True)
    fd = os.open(str(path), os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w", encoding="utf-8") as stream:
        json.dump(config, stream, ensure_ascii=False)


if __name__ == "__main__":
    try:
        render(sys.argv[1], sys.argv[2], os.environ)
    except (OSError, ValueError, KeyError, TypeError):
        sys.exit("Could not render config.prod.json. Check MONGO_PASSWORD, template structure and variable names.")

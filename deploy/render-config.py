"""Render the production template with individual Actions variables and secrets."""

import json
import os
from pathlib import Path
import re
import sys


PLACEHOLDER = "\u5077apikey\u7684\u66ff\u6211\u6321\u707e"
SECRET_FIELDS = {
    "MONGO_PASSWORD": ("mongo", "password"),
    "JWT_SECRET": ("auth", "access_token_secret"),
    "OSS_ACCESS_KEY_ID": ("oss", "access_key_id"),
    "OSS_ACCESS_KEY_SECRET": ("oss", "access_key_secret"),
    "DASHSCOPE_API_KEY": ("dashscope", "api_key"),
    "DEEPSEEK_API_KEY": ("deepseek", "api_key"),
    "SMS_ACCESS_KEY_ID": ("auth", "phone_code", "access_key_id"),
    "SMS_ACCESS_KEY_SECRET": ("auth", "phone_code", "access_key_secret"),
    "SMS_SECURITY_TOKEN": ("auth", "phone_code", "security_token"),
}
VARIABLE_NAMES = {
    "MONGO_SERVICE", "MONGO_PORT", "MONGO_USERNAME", "MONGO_DATABASE",
    "MONGO_AUTH_SOURCE", "SMS_SCHEME_NAME", "SMS_SIGN_NAME", "SMS_TEMPLATE_CODE",
}


def render(source, output, environ):
    """Resolve whole-value placeholders without shell or raw JSON substitution."""
    config = json.loads(Path(source).read_text(encoding="utf-8-sig"))

    def resolve(value):
        if isinstance(value, dict):
            return {key: resolve(item) for key, item in value.items()}
        if isinstance(value, list):
            return [resolve(item) for item in value]
        if isinstance(value, str) and re.fullmatch(r"\$\{[A-Z_]+\}", value):
            name = value[2:-1]
            if name not in VARIABLE_NAMES:
                raise ValueError("Unsupported production variable name.")
            return environ.get(name, "")
        return value

    config = resolve(config)
    for name, keys in SECRET_FIELDS.items():
        target = config
        for key in keys[:-1]:
            target = target[key]
        # Empty Secrets preserve the explicit placeholder, except optional STS.
        target[keys[-1]] = environ.get(name) or ("" if name == "SMS_SECURITY_TOKEN" else PLACEHOLDER)
    path = Path(output)
    path.parent.mkdir(parents=True, exist_ok=True)
    fd = os.open(str(path), os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w", encoding="utf-8") as stream:
        json.dump(config, stream, ensure_ascii=False)


if __name__ == "__main__":
    try:
        render(sys.argv[1], sys.argv[2], os.environ)
    except (OSError, ValueError, KeyError, TypeError):
        sys.exit("Could not render config.prod.json. Check its structure and variable names.")

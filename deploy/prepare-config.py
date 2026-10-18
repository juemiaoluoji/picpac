"""Prepare runtime config on the server; never return credential values to CI."""

import json
import os
from pathlib import Path
import re
import secrets
import sys
from urllib.parse import parse_qsl, quote, unquote, urlencode, urlsplit


PLACEHOLDER = "\u5077apikey\u7684\u66ff\u6211\u6321\u707e"


def mongo_uri(mongo, previous):
    """Build the application URI from split settings; reuse saved values on upgrade."""
    if "service" not in mongo:
        # Keep direct server-side configuration usable for manual deployments.
        uri = mongo.get("uri")
        if not isinstance(uri, str) or not uri.startswith(("mongodb://", "mongodb+srv://")):
            sys.exit("config.json mongo.uri must start with mongodb:// or mongodb+srv://.")
        return uri

    previous_mongo = previous.get("mongo", {})
    if not isinstance(previous_mongo, dict):
        previous_mongo = {}
    try:
        saved = urlsplit(previous_mongo.get("uri", ""))
        old_port = saved.port
    except (AttributeError, TypeError, ValueError):
        saved, old_port = urlsplit(""), None
    old_valid = saved.scheme == "mongodb" and PLACEHOLDER not in unquote(saved.geturl())
    host = mongo.pop("service") or (saved.hostname if old_valid else "")
    port = str(mongo.pop("port", "") or (old_port if old_valid else "") or "27017")
    same_endpoint = old_valid and host == saved.hostname and port == str(old_port or 27017)
    username = mongo.pop("username", "") or (unquote(saved.username or "") if same_endpoint else "")
    password = mongo.pop("password", "")
    if not password or password == PLACEHOLDER:
        password = unquote(saved.password or "") if same_endpoint and username == unquote(saved.username or "") else ""
    database = mongo.get("database")
    if not isinstance(host, str) or not re.fullmatch(r"[a-zA-Z0-9](?:[a-zA-Z0-9.-]*[a-zA-Z0-9])?", host):
        sys.exit("Set MONGO_SERVICE to the Mongo Service DNS name.")
    if host in ("localhost", "127.0.0.1"):
        sys.exit("MONGO_SERVICE must not be a loopback address.")
    if not re.fullmatch(r"[0-9]{1,5}", port) or not 1 <= int(port) <= 65535:
        sys.exit("MONGO_PORT must be an internal Service port between 1 and 65535.")
    if not isinstance(username, str) or not username or not isinstance(password, str) or not password:
        sys.exit("Set MONGO_USERNAME and the MONGO_PASSWORD Secret for first deployment.")
    if not isinstance(database, str) or not database:
        sys.exit("MONGO_DATABASE is required.")
    # Preserve existing replica-set / TLS options when updating the same endpoint.
    options = dict(parse_qsl(saved.query)) if same_endpoint else {}
    configured_options = mongo.pop("options", {})
    if not isinstance(configured_options, dict):
        sys.exit("config.prod.json mongo.options must be an object.")
    options.update({key: str(value) for key, value in configured_options.items() if value != ""})
    options.setdefault("authSource", "admin")
    credentials = quote(username, safe="") + ":" + quote(password, safe="")
    uri = f"mongodb://{credentials}@{host}:{port}/{quote(database, safe='')}"
    return uri + ("?" + urlencode(options) if options else "")


def prepare(source, output, destination):
    try:
        config = json.loads(Path(source).read_text(encoding="utf-8-sig"))
    except (OSError, UnicodeDecodeError, ValueError):
        sys.exit("Repository config must be a UTF-8 JSON object.")
    if not isinstance(config, dict):
        sys.exit("Repository config must contain a JSON object.")
    if not isinstance(config.get("server"), dict):
        sys.exit("config.json is missing the server object.")
    port = config["server"].get("port")
    if not isinstance(port, str) or not re.fullmatch(r"[0-9]{1,5}", port) or not 1 <= int(port) <= 65535:
        sys.exit('config.json server.port must be a numeric string, for example "9090".')
    if not isinstance(config.get("mongo"), dict):
        sys.exit("config.json is missing the mongo object.")
    try:
        previous = json.loads(Path(destination).read_text(encoding="utf-8-sig"))
    except (OSError, UnicodeDecodeError, ValueError):
        previous = {}
    if not isinstance(previous, dict):
        previous = {}
    uri = mongo_uri(config["mongo"], previous)
    try:
        parsed = urlsplit(uri)
        loopback = parsed.hostname in ["localhost", "127.0.0.1", "::1"]
    except ValueError:
        sys.exit("Invalid Mongo URI.")
    if loopback:
        sys.exit("Mongo URI must use a Service address, not loopback.")
    if PLACEHOLDER in unquote(uri):
        sys.exit("Mongo credentials must not contain placeholder values.")
    auth = config.setdefault("auth", {})
    if not auth.get("access_token_secret") or auth.get("access_token_secret") == PLACEHOLDER:
        old_key = previous.get("auth", {}).get("access_token_secret", "")
        if old_key and old_key != PLACEHOLDER:
            auth["access_token_secret"] = old_key
        else:
            key_path = Path(destination).parent / ".picpac-jwt-secret"
            key_path.parent.mkdir(parents=True, exist_ok=True)
            if not key_path.exists():
                fd = os.open(str(key_path), os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
                with os.fdopen(fd, "w", encoding="utf-8") as stream:
                    stream.write(secrets.token_urlsafe(48))
            auth["access_token_secret"] = key_path.read_text(encoding="utf-8").strip()

    config["mongo"]["uri"] = uri
    Path(output).write_text(json.dumps(config, ensure_ascii=False), encoding="utf-8")
    print(port)


if __name__ == "__main__":
    prepare(*sys.argv[1:])

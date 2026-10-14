"""Prepare runtime config on the server; never return credential values to CI."""

import base64
import json
import os
from pathlib import Path
import re
import secrets
import subprocess
import sys
from urllib.parse import quote, unquote, urlsplit, urlunsplit


PLACEHOLDER = "\u5077apikey\u7684\u66ff\u6211\u6321\u707e"


def kube_json(binary, *args):
    command = [binary, "kubectl", *args]
    if os.name == "nt":
        command.insert(0, os.environ["BASH_FOR_TESTS"])
    result = subprocess.run(command, capture_output=True, text=True, encoding="utf-8")
    if result.returncode:
        sys.exit("Could not read existing Mongo deployment metadata.")
    try:
        return json.loads(result.stdout)
    except ValueError:
        sys.exit("Existing Mongo metadata was not valid JSON.")


def env_values(binary, namespace, container):
    values = {}
    for entry in container.get("envFrom", []):
        for ref_key, resource in [("secretRef", "secret"), ("configMapRef", "configmap")]:
            if ref_key in entry:
                obj = kube_json(binary, "-n", namespace, "get", resource, entry[ref_key]["name"], "-o", "json")
                for key, value in obj.get("data", {}).items():
                    values[key] = base64.b64decode(value).decode() if resource == "secret" else value
    for entry in container.get("env", []):
        if "value" in entry:
            values[entry["name"]] = entry["value"]
        for ref_key, resource in [("secretKeyRef", "secret"), ("configMapKeyRef", "configmap")]:
            ref = entry.get("valueFrom", {}).get(ref_key)
            if ref:
                obj = kube_json(binary, "-n", namespace, "get", resource, ref["name"], "-o", "json")
                value = obj.get("data", {}).get(ref["key"], "")
                values[entry["name"]] = base64.b64decode(value).decode() if resource == "secret" else value
    return values


def prepare(source, output, destination, binary, mongo_node_port):
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
    uri = config["mongo"].get("uri")
    if not isinstance(uri, str) or not uri.startswith(("mongodb://", "mongodb+srv://")):
        sys.exit("config.json mongo.uri must start with mongodb:// or mongodb+srv://.")

    try:
        previous = json.loads(Path(destination).read_text(encoding="utf-8-sig"))
    except (OSError, UnicodeDecodeError, ValueError):
        previous = {}
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

    if PLACEHOLDER in unquote(uri):
        old_uri = previous.get("mongo", {}).get("uri", "")
        if old_uri.startswith(("mongodb://", "mongodb+srv://")) and PLACEHOLDER not in unquote(old_uri):
            uri = old_uri
    parsed = urlsplit(uri)
    loopback = parsed.hostname in ["localhost", "127.0.0.1", "::1"]
    needs_credentials = PLACEHOLDER in unquote(uri) or not parsed.username
    if loopback or PLACEHOLDER in unquote(uri):
        services = kube_json(binary, "get", "services", "-A", "-o", "json")
        matches = [(service, item) for service in services.get("items", [])
                   for item in service.get("spec", {}).get("ports", [])
                   if item.get("nodePort") == int(mongo_node_port)]
        if len(matches) != 1:
            sys.exit("Mongo URI uses loopback and no unique Mongo Service matches MONGO_NODE_PORT.")
        service, item = matches[0]
        namespace = service["metadata"]["namespace"]
        host = f"{service['metadata']['name']}.{namespace}.svc.cluster.local:{item['port']}"
        userinfo, separator, _ = parsed.netloc.rpartition("@")
        if needs_credentials:
            selector = ",".join(f"{key}={value}" for key, value in service.get("spec", {}).get("selector", {}).items())
            if not selector:
                sys.exit("Mongo Service needs a selector to discover existing credentials.")
            pods = kube_json(binary, "-n", namespace, "get", "pods", "-l", selector, "-o", "json")
            userinfo, separator = "", ""
            for pod in pods.get("items", []):
                for container in pod.get("spec", {}).get("containers", []):
                    values = env_values(binary, namespace, container)
                    username = values.get("MONGO_INITDB_ROOT_USERNAME") or values.get("MONGODB_ROOT_USER")
                    password = values.get("MONGO_INITDB_ROOT_PASSWORD") or values.get("MONGODB_ROOT_PASSWORD")
                    if username and password:
                        userinfo = quote(username, safe="") + ":" + quote(password, safe="")
                        separator = "@"
                        break
                if separator:
                    break
            if not separator:
                sys.exit("Existing Mongo credentials could not be discovered. Configure mongo.uri before starting the service.")
        uri = urlunsplit((parsed.scheme, (userinfo + "@" if separator else "") + host, parsed.path, parsed.query, parsed.fragment))
        print("Mongo endpoint resolved to its existing Service.", file=sys.stderr)
    config["mongo"]["uri"] = uri
    Path(output).write_text(json.dumps(config, ensure_ascii=False), encoding="utf-8")
    print(port)


if __name__ == "__main__":
    prepare(*sys.argv[1:])

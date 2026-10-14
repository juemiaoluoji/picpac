#!/usr/bin/env bash
# Deploy picpac from a published image and an existing server-side config file.
set -euo pipefail

image="${1:?image is required}"
config_file="${2:?config path is required}"
platform="${3:?platform is required}"
node_port="${4:?NodePort is required}"
credentials_file="${5:?registry credentials file is required}"
template_file="${6:?deployment template is required}"
incoming_config="${7:-}"
mongo_node_port="${8:-32017}"
k3s_bin="${K3S_BIN:-/usr/local/bin/k3s}"
python_bin="${PYTHON_BIN:-python3}"

[[ "$(id -u)" == 0 ]] || { echo 'Run the deployment with sudo -n -i.' >&2; exit 1; }
[[ "$image" =~ ^[a-z0-9.-]+/[a-z0-9._-]+/[a-z0-9._-]+:([a-f0-9]{7}|sha-[a-f0-9]{40})$ ]] || { echo 'Invalid published image address.' >&2; exit 1; }
case "$platform" in
  linux/amd64) arch=amd64 ;;
  linux/arm64) arch=arm64 ;;
  *) echo 'Unsupported platform.' >&2; exit 1 ;;
esac
[[ "$node_port" =~ ^[0-9]{5}$ ]] && (( 10#$node_port >= 30000 && 10#$node_port <= 32767 )) || { echo 'NodePort must be between 30000 and 32767.' >&2; exit 1; }
config_source="${incoming_config:-$config_file}"
[[ -r "$config_source" && -r "$credentials_file" && -r "$template_file" ]] || { echo 'Config, credentials or deployment template is missing.' >&2; exit 1; }
[[ -x "$k3s_bin" ]] || { echo 'k3s executable was not found.' >&2; exit 1; }

# kube uses k3s's root-owned admin kubeconfig, without exposing it to GitHub.
kube() { "$k3s_bin" kubectl "$@"; }

work_dir="$(mktemp -d /tmp/picpac-k3s.XXXXXX)"
trap 'rm -rf -- "$work_dir"' EXIT
runtime_config="$work_dir/config.json"
port="$("$python_bin" - "$config_source" "$runtime_config" <<'PY'
import json, re, sys
from urllib.parse import unquote
try:
    with open(sys.argv[1], encoding='utf-8-sig') as stream:
        config = json.load(stream)
except json.JSONDecodeError as error:
    sys.exit(f'config.json is not valid JSON: {error.msg} (line {error.lineno}, column {error.colno}).')
except (OSError, UnicodeDecodeError):
    sys.exit('config.json must be a readable UTF-8 JSON file.')
if not isinstance(config, dict):
    sys.exit('config.json must contain a JSON object.')
server = config.get('server')
if not isinstance(server, dict):
    sys.exit('config.json is missing the server object.')
port = server.get('port')
if not isinstance(port, str) or not re.fullmatch(r'[0-9]{1,5}', port) or not 1 <= int(port) <= 65535:
    sys.exit('config.json server.port must be a numeric string, for example "9090".')
mongo = config.get('mongo')
if not isinstance(mongo, dict):
    sys.exit('config.json is missing the mongo object.')
uri = mongo.get('uri')
if not isinstance(uri, str) or not uri.startswith(('mongodb://', 'mongodb+srv://')):
    sys.exit('config.json mongo.uri must start with mongodb:// or mongodb+srv://.')
placeholder = '\u5077apikey\u7684\u66ff\u6211\u6321\u707e'
auth = config.get('auth', {})
if placeholder in unquote(uri) or (isinstance(auth, dict) and auth.get('access_token_secret') == placeholder):
    sys.exit('Runtime Mongo/JWT credentials still contain repository placeholders. Configure PICPAC_CONFIG_OVERRIDES before deployment.')
hosts = uri.split('://', 1)[1].split('/', 1)[0].rsplit('@', 1)[-1]
if not hosts:
    sys.exit('config.json mongo.uri must include a host.')
with open(sys.argv[2], 'w', encoding='utf-8') as stream:
    json.dump(config, stream, ensure_ascii=False)
print(int(port))
PY
)"
needs_mongo_service="$("$python_bin" - "$runtime_config" <<'PY'
import json, re, sys
with open(sys.argv[1], encoding='utf-8') as stream:
    uri = json.load(stream)['mongo']['uri']
hosts = uri.split('://', 1)[1].split('/', 1)[0].rsplit('@', 1)[-1]
print('yes' if re.search(r'(^|,)(localhost|127\.[0-9.]+|\[::1\])(?=[:,?]|$)', hosts, re.I) else 'no')
PY
)"
if [[ "$needs_mongo_service" == yes ]]; then
  [[ "$mongo_node_port" =~ ^[0-9]{5}$ ]] || { echo 'Invalid Mongo NodePort.' >&2; exit 1; }
  kube get services -A -o json > "$work_dir/services.json"
  "$python_bin" - "$runtime_config" "$work_dir/services.json" "$mongo_node_port" <<'PY'
import json, sys
from urllib.parse import urlsplit, urlunsplit
with open(sys.argv[1], encoding='utf-8') as stream:
    config = json.load(stream)
with open(sys.argv[2], encoding='utf-8') as stream:
    services = json.load(stream)
matches = [(service, port) for service in services.get('items', [])
           for port in service.get('spec', {}).get('ports', [])
           if port.get('nodePort') == int(sys.argv[3])]
if len(matches) != 1:
    sys.exit('Mongo URI uses loopback and no unique Mongo Service matches MONGO_NODE_PORT. Configure a reachable mongo.uri override.')
service, port = matches[0]
host = f"{service['metadata']['name']}.{service['metadata']['namespace']}.svc.cluster.local:{port['port']}"
parsed = urlsplit(config['mongo']['uri'])
credentials, separator, _ = parsed.netloc.rpartition('@')
netloc = (credentials + '@' if separator else '') + host
config['mongo']['uri'] = urlunsplit((parsed.scheme, netloc, parsed.path, parsed.query, parsed.fragment))
with open(sys.argv[1], 'w', encoding='utf-8') as stream:
    json.dump(config, stream, ensure_ascii=False)
print('Mongo endpoint uses its in-cluster Service: ' + host)
PY
fi
if [[ -n "$incoming_config" ]]; then
  [[ "$config_file" == /* ]] || { echo 'Config destination must be an absolute server path.' >&2; exit 1; }
  mkdir -p -- "$(dirname "$config_file")"
  staged_config="$(mktemp "${config_file}.XXXXXX")"
  install -m 600 "$runtime_config" "$staged_config"
  mv -f -- "$staged_config" "$config_file"
  echo 'Synchronized repository config and runtime overrides to the server.'
fi
config_sha="$(sha256sum "$runtime_config" | cut -d ' ' -f1)"

kube get nodes >/dev/null
kube create namespace picpac --dry-run=client -o yaml | kube apply -f -
kube -n picpac create secret generic tcr-pull \
  --type=kubernetes.io/dockerconfigjson --from-file=".dockerconfigjson=$credentials_file" \
  --dry-run=client -o yaml | kube apply -f -
kube -n picpac create secret generic picpac-config \
  --from-file="config.json=$runtime_config" --dry-run=client -o yaml | kube apply -f -

existing="$(kube -n picpac get deployment picpac --ignore-not-found -o name)"
if [[ -z "$existing" ]]; then
  sed -e "s|__IMAGE__|$image|g" -e "s|__PORT__|$port|g" \
    -e "s|__ARCH__|$arch|g" -e "s|__CONFIG_SHA__|$config_sha|g" \
    "$template_file" > "$work_dir/deployment.yaml"
  kube apply -f "$work_dir/deployment.yaml"
  echo 'Created picpac Deployment.'
else
  container="$(kube -n picpac get deployment picpac -o 'jsonpath={.spec.template.spec.containers[?(@.name=="picpac")].name}')"
  [[ "$container" == picpac ]] || { echo 'Existing Deployment must have a container named picpac.' >&2; exit 1; }
  # Strategic merge preserves existing replicas, resources, labels and other containers.
  cat > "$work_dir/patch.json" <<JSON
{"spec":{"template":{"metadata":{"annotations":{"picpac/config-sha256":"$config_sha"}},"spec":{"nodeSelector":{"kubernetes.io/arch":"$arch"},"imagePullSecrets":[{"name":"tcr-pull"}],"containers":[{"name":"picpac","image":"$image","env":[{"name":"PICPAC_CONFIG","value":"/app/config/config.json"}],"volumeMounts":[{"name":"picpac-config","mountPath":"/app/config/config.json","subPath":"config.json","readOnly":true}]}],"volumes":[{"name":"picpac-config","secret":{"secretName":"picpac-config","defaultMode":292}}]}}}}
JSON
  kube -n picpac patch deployment picpac --type=strategic --patch-file "$work_dir/patch.json"
  echo 'Updated picpac image and mounted configuration.'
fi

service="$(kube -n picpac get service picpac --ignore-not-found -o name)"
if [[ -z "$service" ]]; then
  selector="$(kube -n picpac get deployment picpac -o 'jsonpath={.spec.selector.matchLabels}')"
  [[ "$selector" != '' && "$selector" != '{}' ]] || { echo 'Deployment needs matchLabels for its Service.' >&2; exit 1; }
  cat > "$work_dir/service.json" <<JSON
{"apiVersion":"v1","kind":"Service","metadata":{"name":"picpac","namespace":"picpac"},"spec":{"type":"NodePort","selector":$selector,"ports":[{"name":"http","port":$port,"targetPort":$port,"nodePort":$node_port}]}}
JSON
  kube apply -f "$work_dir/service.json"
fi

if ! kube -n picpac rollout status deployment/picpac --timeout=180s; then
  kube -n picpac get pods -o wide
  echo 'Deployment did not become ready. Check pod logs and Mongo connectivity.' >&2
  exit 1
fi
kube -n picpac get deployment picpac
kube -n picpac get service picpac

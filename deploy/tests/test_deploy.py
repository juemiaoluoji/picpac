"""Check deployment decisions without connecting to a real Kubernetes cluster."""

import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
IMAGE = "ccr.ccs.tencentyun.com/picpac/server:123abcd"


class DeploymentTests(unittest.TestCase):
    def write_script(self, path, content):
        with path.open("w", encoding="utf-8", newline="\n") as stream:
            stream.write(content)
        path.chmod(0o755)

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="picpac-deploy-test-")
        self.addCleanup(self.temp.cleanup)
        self.work = Path(self.temp.name)
        self.capture = self.work / "capture"
        self.capture.mkdir()
        self.calls = self.work / "calls.log"
        self.config = self.work / "config.json"
        self.config.write_text(json.dumps({
            "server": {"port": "9090"},
            "mongo": {"uri": "mongodb://mongo.example:27017/picpac?replicaSet=rs0"},
        }), encoding="utf-8")
        self.credentials = self.work / "tcr.json"
        self.credentials.write_text('{"auths":{}}', encoding="utf-8")
        self.mock = self.work / "k3s"
        self.write_script(self.mock, """#!/usr/bin/env bash
set -eu
printf '%s ' "$@" >> "$FAKE_CALLS"
printf '\\n' >> "$FAKE_CALLS"
args="$*"
case "$args" in
  *'get services -A -o json'*)
    if [[ "${FAKE_MONGO_SERVICE:-0}" == 1 ]]; then
      echo '{"items":[{"metadata":{"name":"mongodb","namespace":"data"},"spec":{"selector":{"app":"mongodb"},"ports":[{"port":27017,"nodePort":32017}]}}]}'
    else echo '{"items":[]}'; fi ;;
  *'get pods -l '*)
    echo '{"items":[{"spec":{"containers":[{"env":[{"name":"MONGO_INITDB_ROOT_USERNAME","value":"example-user"},{"name":"MONGO_INITDB_ROOT_PASSWORD","value":"test-only-password"}]}]}}]}' ;;
  *'get deployment picpac --ignore-not-found'*)
    if [[ "${FAKE_EXISTING:-0}" == 1 ]]; then echo deployment.apps/picpac; fi ;;
  *'get service picpac --ignore-not-found'*)
    if [[ "${FAKE_SERVICE:-0}" == 1 ]]; then echo service/picpac; fi ;;
  *'containers['*) echo "${FAKE_CONTAINER:-picpac}" ;;
  *'spec.selector.matchLabels'*) echo '{"app":"picpac"}' ;;
  *'apply -f -'*) cat >/dev/null ;;
  *'apply -f '*|*'--patch-file '*)
    last="${!#}"
    cp "$last" "$FAKE_CAPTURE/$(basename "$last")" ;;
  *'rollout status'*) exit "${FAKE_ROLLOUT_EXIT:-0}" ;;
esac
""")
        self.write_script(self.work / "id", "#!/usr/bin/env bash\nprintf '0\\n'\n")
        self.env = os.environ.copy()
        self.env.update({
            "K3S_BIN": self.mock.as_posix(),
            "PYTHON_BIN": Path(sys.executable).as_posix(),
            "FAKE_CALLS": self.calls.as_posix(),
            "FAKE_CAPTURE": self.capture.as_posix(),
            "PATH": self.work.as_posix() + os.pathsep + self.env["PATH"],
        })
        mock_path = self.work.as_posix()
        if os.name == "nt":
            mock_path = "/" + mock_path[0].lower() + mock_path[2:]
        self.env["MOCK_BIN"] = mock_path
        self.bash = shutil.which("bash")
        if os.name == "nt":
            self.bash = r"C:\Program Files\Git\bin\bash.exe"
        if not self.bash:
            self.skipTest("bash is required")
        self.env["BASH_FOR_TESTS"] = self.bash

    def run_deploy(self, image=IMAGE, node_port="32090", incoming=None, destination=None):
        args = [
            self.bash, "-c", 'export PATH="$MOCK_BIN:$PATH"; exec bash "$@"', "test-deploy",
            (ROOT / "deploy/deploy-k3s.sh").as_posix(),
            image, destination or self.config.as_posix(), "linux/amd64", node_port,
            self.credentials.as_posix(), (ROOT / "deploy/picpac.yaml").as_posix(),
        ]
        if incoming:
            args.extend([incoming, "32017"])
        return subprocess.run(args, env=self.env, capture_output=True, text=True, encoding="utf-8", errors="replace")

    def test_first_deployment_creates_workload_and_service(self):
        result = self.run_deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        deployment = (self.capture / "deployment.yaml").read_text()
        self.assertIn(IMAGE, deployment)
        self.assertIn("/app/config/config.json", deployment)
        self.assertNotIn("__PORT__", deployment)
        service = json.loads((self.capture / "service.json").read_text())
        self.assertEqual(service["spec"]["ports"][0]["nodePort"], 32090)
        self.assertEqual(service["spec"]["selector"], {"app": "picpac"})

    def test_update_preserves_existing_service_and_replica_settings(self):
        self.env.update(FAKE_EXISTING="1", FAKE_SERVICE="1")
        result = self.run_deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        patch = json.loads((self.capture / "patch.json").read_text())
        self.assertNotIn("replicas", patch["spec"])
        container = patch["spec"]["template"]["spec"]["containers"][0]
        self.assertEqual(container["image"], IMAGE)
        self.assertNotIn("resources", container)
        self.assertFalse((self.capture / "service.json").exists())

    def test_missing_config_fails_before_cluster_changes(self):
        self.config.unlink()
        result = self.run_deploy()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.calls.exists())

    def test_loopback_mongo_fails_before_cluster_changes(self):
        for host in ["localhost", "127.0.0.1", "[::1]"]:
            with self.subTest(host=host):
                self.config.write_text(json.dumps({
                    "server": {"port": "9090"}, "mongo": {"uri": f"mongodb://{host}:32017/picpac"},
                }))
                result = self.run_deploy()
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("loopback", result.stderr)
                self.assertNotIn("apply", self.calls.read_text())

    def test_invalid_image_and_nodeport_are_rejected(self):
        for image, port in [(IMAGE + ";whoami", "32090"), (IMAGE, "22")]:
            with self.subTest(image=image, port=port):
                result = self.run_deploy(image, port)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(self.calls.exists())

    def test_config_errors_report_fields_without_printing_values(self):
        for config, message in [
            ([], "JSON object"),
            ({}, "server object"),
            ({"server": {"port": 9090}}, "server.port"),
            ({"server": {"port": "9090"}}, "mongo object"),
            ({"server": {"port": "9090"}, "mongo": {"uri": "private-value"}}, "mongo.uri"),
        ]:
            with self.subTest(message=message):
                self.config.write_text(json.dumps(config))
                result = self.run_deploy()
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(message, result.stderr)
                self.assertNotIn("private-value", result.stderr)
                self.assertFalse(self.calls.exists())

    def test_utf8_bom_is_normalized_without_changing_source(self):
        content = self.config.read_bytes()
        self.config.write_bytes(b'\xef\xbb\xbf' + content)
        result = self.run_deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.config.read_bytes(), b'\xef\xbb\xbf' + content)

    def test_repository_config_replaces_invalid_server_file(self):
        destination = self.work / "server-config.json"
        destination.write_text("invalid old config")
        shell_path = destination.as_posix()
        if os.name == "nt":
            shell_path = "/" + shell_path[0].lower() + shell_path[2:]
        result = self.run_deploy(incoming=self.config.as_posix(), destination=shell_path)
        self.assertEqual(result.returncode, 0, result.stderr)
        config = json.loads(destination.read_text())
        self.assertEqual(config["server"], {"port": "9090"})
        self.assertGreater(len(config["auth"]["access_token_secret"]), 40)

    def test_loopback_mongo_resolves_only_the_selected_service(self):
        self.env["FAKE_MONGO_SERVICE"] = "1"
        self.config.write_text(json.dumps({
            "server": {"port": "9090"},
            "mongo": {"uri": "mongodb://localhost:32017/picpac?replicaSet=rs0"},
        }))
        destination = self.work / "server-config.json"
        shell_path = destination.as_posix()
        if os.name == "nt":
            shell_path = "/" + shell_path[0].lower() + shell_path[2:]
        result = self.run_deploy(incoming=self.config.as_posix(), destination=shell_path)
        self.assertEqual(result.returncode, 0, result.stderr)
        config = json.loads(destination.read_text())
        self.assertEqual(config["mongo"]["uri"], "mongodb://example-user:test-only-password@mongodb.data.svc.cluster.local:27017/picpac?replicaSet=rs0")

    def test_jwt_is_generated_on_server_and_preserved(self):
        placeholder = "\u5077apikey\u7684\u66ff\u6211\u6321\u707e"
        self.config.write_text(json.dumps({
            "server": {"port": "9090"},
            "mongo": {"uri": "mongodb://mongo.example:27017/picpac"},
            "auth": {"access_token_secret": placeholder},
        }))
        destination = self.work / "server-config.json"
        shell_path = destination.as_posix()
        if os.name == "nt":
            shell_path = "/" + shell_path[0].lower() + shell_path[2:]
        result = self.run_deploy(incoming=self.config.as_posix(), destination=shell_path)
        self.assertEqual(result.returncode, 0, result.stderr)
        key = json.loads(destination.read_text())["auth"]["access_token_secret"]
        self.assertNotEqual(key, placeholder)
        result = self.run_deploy(incoming=self.config.as_posix(), destination=shell_path)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(destination.read_text())["auth"]["access_token_secret"], key)

    def test_failed_rollout_is_reported_as_failure(self):
        self.env["FAKE_ROLLOUT_EXIT"] = "1"
        result = self.run_deploy()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("did not become ready", result.stderr)

    def test_legacy_published_tag_can_be_redeployed(self):
        result = self.run_deploy("ccr.ccs.tencentyun.com/picpac/server:sha-" + "a" * 40)
        self.assertEqual(result.returncode, 0, result.stderr)


if __name__ == "__main__":
    unittest.main()

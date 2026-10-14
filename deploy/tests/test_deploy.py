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

    def run_deploy(self, image=IMAGE, node_port="32090"):
        return subprocess.run([
            self.bash, "-c", 'export PATH="$MOCK_BIN:$PATH"; exec bash "$@"', "test-deploy",
            (ROOT / "deploy/deploy-k3s.sh").as_posix(),
            image, self.config.as_posix(), "linux/amd64", node_port,
            self.credentials.as_posix(), (ROOT / "deploy/picpac.yaml").as_posix(),
        ], env=self.env, capture_output=True, text=True, encoding="utf-8", errors="replace")

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
                self.assertFalse(self.calls.exists())

    def test_invalid_image_and_nodeport_are_rejected(self):
        for image, port in [(IMAGE + ";whoami", "32090"), (IMAGE, "22")]:
            with self.subTest(image=image, port=port):
                result = self.run_deploy(image, port)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(self.calls.exists())

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

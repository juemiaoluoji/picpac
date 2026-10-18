"""Check the actual production template and its individual Actions inputs."""

import importlib.util
from contextlib import redirect_stdout
import io
import json
from pathlib import Path
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("render_config", ROOT / "deploy/render-config.py")
RENDERER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(RENDERER)
PREPARE_SPEC = importlib.util.spec_from_file_location("prepare_config", ROOT / "deploy/prepare-config.py")
PREPARER = importlib.util.module_from_spec(PREPARE_SPEC)
PREPARE_SPEC.loader.exec_module(PREPARER)


class RenderConfigTests(unittest.TestCase):
    def render(self, values):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "config.json"
            RENDERER.render(ROOT / "config.prod.json", output, values)
            return json.loads(output.read_text(encoding="utf-8"))

    def test_individual_secrets_preserve_json_escaping(self):
        value = 'test-only-value"\\\n${MONGO_SERVICE}'
        config = self.render({
            "MONGO_SERVICE": "mongodb.data.svc.cluster.local", "MONGO_PORT": "27017",
            "MONGO_USERNAME": "example-user", "MONGO_DATABASE": "picpac", "MONGO_AUTH_SOURCE": "admin",
            "MONGO_PASSWORD": value, "OSS_ACCESS_KEY_SECRET": value,
            "DEEPSEEK_API_KEY": value, "SMS_SIGN_NAME": "test-sign",
        })
        self.assertEqual(config["env"], "prod")
        self.assertEqual(config["mongo"]["service"], "mongodb.data.svc.cluster.local")
        self.assertEqual(config["mongo"]["password"], value)
        self.assertEqual(config["oss"]["access_key_secret"], value)
        self.assertEqual(config["deepseek"]["api_key"], value)
        self.assertEqual(config["auth"]["phone_code"]["sign_name"], "test-sign")

    def test_empty_optional_secrets_leave_placeholders_for_external_services(self):
        config = self.render({"MONGO_PASSWORD": "test-only-password"})
        self.assertEqual(config["mongo"]["service"], "")
        self.assertEqual(config["mongo"]["password"], "test-only-password")
        self.assertEqual(config["auth"]["access_token_secret"], RENDERER.PLACEHOLDER)
        self.assertEqual(config["oss"]["access_key_id"], RENDERER.PLACEHOLDER)
        self.assertEqual(config["auth"]["phone_code"]["security_token"], "")

    def test_missing_or_placeholder_mongo_password_is_rejected_before_writing(self):
        for values in [{}, {"MONGO_PASSWORD": ""}, {"MONGO_PASSWORD": RENDERER.PLACEHOLDER}]:
            with self.subTest(values_present=bool(values)):
                with tempfile.TemporaryDirectory() as directory:
                    output = Path(directory) / "config.json"
                    with self.assertRaisesRegex(ValueError, "MONGO_PASSWORD"):
                        RENDERER.render(ROOT / "config.prod.json", output, values)
                    self.assertFalse(output.exists())

    def test_actual_template_renders_to_application_runtime_config(self):
        with tempfile.TemporaryDirectory() as directory:
            source, output, saved = (Path(directory) / name for name in ["incoming.json", "runtime.json", "server.json"])
            RENDERER.render(ROOT / "config.prod.json", source, {
                "MONGO_SERVICE": "mongodb.data.svc.cluster.local", "MONGO_PORT": "27017",
                "MONGO_USERNAME": "example-user", "MONGO_PASSWORD": "test-only-password", "MONGO_DATABASE": "picpac",
            })
            with redirect_stdout(io.StringIO()) as stdout:
                PREPARER.prepare(source, output, saved)
            config = json.loads(output.read_text(encoding="utf-8"))
            self.assertEqual(stdout.getvalue(), "9090\n")
            self.assertEqual(config["env"], "prod")
            self.assertEqual(config["mongo"]["uri"], "mongodb://example-user:test-only-password@mongodb.data.svc.cluster.local:27017/picpac?authSource=admin")
            self.assertEqual(set(config["mongo"]), {"uri", "database", "connect_timeout_seconds", "max_pool_size"})
            self.assertGreater(len(config["auth"]["access_token_secret"]), 40)


if __name__ == "__main__":
    unittest.main()

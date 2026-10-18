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
            "MONGO_AUTH_SOURCE": "admin",
            "MONGO_PASSWORD": value, "OSS_ACCESS_KEY_SECRET": value,
            "DEEPSEEK_API_KEY": value,
        })
        self.assertEqual(config["env"], "prod")
        self.assertEqual(config["mongo"]["service"], "mongo.picpac.svc.cluster.local")
        self.assertEqual(config["mongo"]["password"], value)
        self.assertEqual(config["oss"]["access_key_secret"], value)
        self.assertEqual(config["deepseek"]["api_key"], value)

    def test_fixed_mongo_and_sms_settings_ignore_old_actions_variables(self):
        names = ["MONGO_SERVICE", "MONGO_PORT", "MONGO_USERNAME", "MONGO_DATABASE", "SMS_SIGN_NAME", "SMS_TEMPLATE_CODE", "SMS_SCHEME_NAME"]
        config = self.render({**{name: "obsolete-variable-value" for name in names}, "MONGO_PASSWORD": "test-only-password"})
        self.assertEqual({key: config["mongo"][key] for key in ["service", "port", "username", "database"]}, {
            "service": "mongo.picpac.svc.cluster.local", "port": "27017", "username": "admin", "database": "picpac",
        })
        self.assertEqual({key: config["auth"]["phone_code"][key] for key in ["sign_name", "template_code", "scheme_name"]}, {
            "sign_name": "\u6052\u521b\u8054\u4f17", "template_code": "100001", "scheme_name": "",
        })

    def test_every_secret_placeholder_has_a_matching_actions_input(self):
        values = {name: "test-only-" + name for name in RENDERER.SECRET_NAMES}
        config = self.render({**values, "MONGO_AUTH_SOURCE": "users"})
        template = json.loads((ROOT / "config.prod.json").read_text(encoding="utf-8"))
        found = set()

        def check(source, result):
            if isinstance(source, dict):
                for key in source:
                    check(source[key], result[key])
            elif isinstance(source, list):
                for item, rendered in zip(source, result):
                    check(item, rendered)
            elif isinstance(source, str) and source.startswith("${"):
                name = source[2:-1]
                found.add(name)
                self.assertEqual(result, (values | {"MONGO_AUTH_SOURCE": "users"})[name])

        check(template, config)
        self.assertEqual(found, RENDERER.SECRET_NAMES | RENDERER.VARIABLE_NAMES)
        workflow = (ROOT / ".github/workflows/deploy-k3s.yml").read_text(encoding="utf-8")
        for name in RENDERER.SECRET_NAMES:
            self.assertIn(name + ": ${{ secrets." + name + " }}", workflow)
        for name in RENDERER.VARIABLE_NAMES:
            self.assertIn(name + ": ${{ vars." + name + " }}", workflow)

    def test_unknown_placeholder_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            source, output = Path(directory) / "template.json", Path(directory) / "output.json"
            source.write_text('{"api_key":"${UNKNOWN_API_KEY}"}')
            with self.assertRaisesRegex(ValueError, "Unsupported"):
                RENDERER.render(source, output, {"MONGO_PASSWORD": "test-only-password"})
            self.assertFalse(output.exists())

    def test_empty_optional_secrets_leave_placeholders_for_external_services(self):
        config = self.render({"MONGO_PASSWORD": "test-only-password"})
        self.assertEqual(config["mongo"]["service"], "mongo.picpac.svc.cluster.local")
        self.assertEqual(config["mongo"]["password"], "test-only-password")
        self.assertEqual(config["auth"]["access_token_secret"], RENDERER.PLACEHOLDER)
        self.assertEqual(config["oss"]["access_key_id"], RENDERER.PLACEHOLDER)
        self.assertEqual(config["dashscope"]["api_key"], RENDERER.PLACEHOLDER)
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
                "MONGO_PASSWORD": "test-only-password",
            })
            with redirect_stdout(io.StringIO()) as stdout:
                PREPARER.prepare(source, output, saved)
            config = json.loads(output.read_text(encoding="utf-8"))
            self.assertEqual(stdout.getvalue(), "9090\n")
            self.assertEqual(config["env"], "prod")
            self.assertEqual(config["mongo"]["uri"], "mongodb://admin:test-only-password@mongo.picpac.svc.cluster.local:27017/picpac?authSource=admin")
            self.assertEqual(set(config["mongo"]), {"uri", "database", "connect_timeout_seconds", "max_pool_size"})
            self.assertGreater(len(config["auth"]["access_token_secret"]), 40)


if __name__ == "__main__":
    unittest.main()

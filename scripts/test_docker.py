"""Docker configuration regressions; no daemon or third-party Python packages needed."""

import itertools
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
APPS = ("api", "cron", "admin", "embedder")


class DockerConfigTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        for filename in ROOT.glob("compose*.yaml"):
            shutil.copy(filename, self.root)
        # Never load the developer's real environment files or credentials.
        self.env = {"PATH": os.environ["PATH"], "HOME": str(self.root)}

    def compose(self, filename, **env):
        result = subprocess.run(
            ["docker", "compose", "--env-file", "/dev/null", "-f", filename,
             "config", "--format", "json"],
            cwd=self.root, env=self.env | env, capture_output=True, text=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        return json.loads(result.stdout)["services"]

    def test_app_env_overrides_reach_workers(self):
        (self.root / ".env").write_text(
            "CODOHUE_OBSERVABILITY_TOKEN=base\nCODOHUE_STREAM_RETENTION_ENABLED=false\n"
        )
        (self.root / ".env.app").write_text(
            "CODOHUE_OBSERVABILITY_TOKEN=app-token\n"
            "CODOHUE_STREAM_RETENTION_ENABLED=true\n"
            "CODOHUE_STREAM_RETENTION_INTERVAL=2m\n"
        )
        services = self.compose("compose.app.yaml")
        for name in ("api", "embedder"):
            env = services[name]["environment"]
            self.assertEqual(env["CODOHUE_OBSERVABILITY_TOKEN"], "app-token")
            self.assertEqual(env["CODOHUE_STREAM_RETENTION_ENABLED"], "true")
            self.assertEqual(env["CODOHUE_STREAM_RETENTION_INTERVAL"], "2m")

    def test_local_ports_and_startup(self):
        services = self.compose("compose.yaml", CODOHUE_API_PORT="3201")
        self.assertEqual(services["api"]["ports"][0]["target"], 3201)
        self.assertEqual(services["admin"]["environment"]["CODOHUE_API_URL"], "http://api:3201")
        for name in ("api", "cron", "embedder"):
            deps = services[name]["depends_on"]
            self.assertEqual(deps["migrate"]["condition"], "service_completed_successfully")
            for infra in ("redis", "qdrant"):
                self.assertEqual(deps[infra]["condition"], "service_healthy")
        self.assertEqual(services["migrate"]["restart"], "no")
        for name in APPS:
            self.assertNotIn("container_name", services[name])
            self.assertEqual(services[name]["stop_grace_period"], "40s")
        for name in ("api", "admin", "embedder"):
            self.assertIn("healthcheck", services[name])

    def test_all_production_infra_combinations(self):
        for enabled in itertools.product((False, True), repeat=3):
            with self.subTest(enabled=enabled):
                profiles = [p for p, yes in zip(("local-db", "local-redis", "local-qdrant"), enabled) if yes]
                services = self.compose(
                    "compose.prod.yaml", COMPOSE_PROFILES=",".join(profiles),
                    CODOHUE_POSTGRES_PASSWORD="test-password", CODOHUE_ADMIN_API_KEY="test-key",
                    CODOHUE_OBSERVABILITY_TOKEN="metrics-token", IMAGE_TAG="test-release",
                    CODOHUE_DATABASE_URL="postgres://db/existing?pool_max_conns=4",
                    CODOHUE_MIGRATION_DATABASE_URL="postgres://db/existing",
                    CODOHUE_REDIS_URL="redis://external:6379", CODOHUE_QDRANT_HOST="external",
                )
                for name in APPS:
                    self.assertTrue(services[name]["image"].endswith(":test-release"))
                    self.assertEqual(services[name]["environment"]["DATABASE_URL"], "postgres://db/existing?pool_max_conns=4")
                self.assertEqual(services["migrate"]["environment"]["DATABASE_URL"], "postgres://db/existing")
                self.assertEqual(services["admin"]["environment"]["CODOHUE_OBSERVABILITY_TOKEN"], "metrics-token")
                for name, yes in zip(("postgres", "redis", "qdrant"), enabled):
                    self.assertEqual(name in services, yes)

    def test_deploy_waits_for_stack_and_propagates_failure(self):
        (self.root / "deploy").mkdir()
        shutil.copy(ROOT / "deploy/deploy.sh", self.root / "deploy/deploy.sh")
        docker = self.root / "docker"
        docker.write_text(
            '#!/bin/sh\nprintf "%s\\n" "$*" >> "$CALL_LOG"\n'
            'case "$*" in *" up "*) exit "$UP_EXIT";; esac\n'
        )
        docker.chmod(0o755)
        for code in (0, 1):
            with self.subTest(code=code):
                log = self.root / f"calls-{code}"
                result = subprocess.run(
                    ["bash", str(self.root / "deploy/deploy.sh")],
                    env=self.env | {"PATH": f"{self.root}:{self.env['PATH']}",
                                    "CALL_LOG": str(log), "UP_EXIT": str(code)},
                    capture_output=True, text=True,
                )
                self.assertEqual(result.returncode, code, result.stderr)
                calls = log.read_text()
                self.assertIn("up --wait --wait-timeout 300", calls)
                self.assertNotIn("prune", calls)
                if code:
                    self.assertIn("ps -a", calls)
                    self.assertIn("logs --tail=50", calls)
                    self.assertNotIn("Deployment complete", result.stdout)

    def test_migration_preserves_url_and_exit_status(self):
        migrate = self.root / "migrate"
        migrate.write_text('#!/bin/sh\nprintf "%s\\n" "$@"\nexit 23\n')
        migrate.chmod(0o755)
        url = "postgres://user:p%40ss@db/existing%27db?sslmode=require&connect_timeout=5"
        command = ["sh", str(ROOT / "docker/migrate-entrypoint.sh")]
        result = subprocess.run(command, env=self.env | {
            "PATH": f"{self.root}:{self.env['PATH']}", "DATABASE_URL": url,
        }, capture_output=True, text=True)
        self.assertEqual(result.returncode, 23)
        self.assertEqual(result.stdout.splitlines(), ["-path", "/migrations", "-database", url, "up"])
        result = subprocess.run(command, env=self.env, capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("DATABASE_URL is required", result.stderr)


if __name__ == "__main__":
    unittest.main()

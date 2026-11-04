"""PR Go-unit validation provisions its own DB without affecting other checks."""
from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[1]


class PRUnitDatabaseRouteTests(unittest.TestCase):
    def test_go_unit_check_uses_ephemeral_db_only_in_selected_check(self):
        workflow = (ROOT / ".github/workflows/ci.yml").read_text()
        self.assertIn("go-unit-tests) bash scripts/pr-unit-database.sh ;;", workflow)
        self.assertEqual(1, workflow.count("bash scripts/pr-unit-database.sh"))

    def test_database_script_preserves_real_test_failure_and_cleans_up(self):
        fixture = (ROOT / "scripts/pr-unit-database.sh").read_text()
        for required in (
            "set -euo pipefail", "postgres:18-alpine",
            "--publish 127.0.0.1:5433:5432",
            "POSTGRES_DB=baseharbor_test", "POSTGRES_PASSWORD=postgres",
            'trap cleanup EXIT', 'docker rm -f "$name"',
            'docker exec "$name" pg_isready',
            "BASEHARBOR_TEST_DATABASE_URL=", "go test ./... -count=1"
        ):
            self.assertIn(required, fixture)


if __name__ == "__main__":
    unittest.main()

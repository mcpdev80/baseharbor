import importlib.util
import json
from pathlib import Path
import unittest

spec=importlib.util.spec_from_file_location("runtime_matrix",Path(__file__).with_name("pr-selected-runtime-matrix.py"))
module=importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class RuntimeMatrix(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.catalog=json.loads(Path(__file__).with_name("release-requirements.json").read_text())

    def test_selected_backup_both_runtimes(self):
        result=module.select({"selected_gates":["atomic/podman/backup-restore","atomic/docker/backup-restore"]},self.catalog)
        self.assertEqual(["atomic/docker/backup-restore","atomic/podman/backup-restore"],
                         [r["id"] for r in result["include"]])
        self.assertFalse(result["deferred"])

    def test_three_native_gates_all_scheduled_with_bounded_parallelism(self):
        gates=["atomic/docker/backup-restore","atomic/podman/backup-restore","atomic/docker/lifecycle"]
        result=module.select({"selected_gates":gates},self.catalog)
        self.assertEqual(set(gates),{r["id"] for r in result["include"]})
        self.assertEqual([],result["unsupported"])
        workflow=(Path(__file__).resolve().parent.parent/".github/workflows/ci.yml").read_text()
        selected=workflow.split("  pr_runtime_selected:",1)[1].split("  pr_selected_final:",1)[0]
        self.assertIn("max-parallel: 2",selected)

    def test_unsupported_not_marked_pass(self):
        result=module.select({"selected_gates":["integration/static/public-contracts"]},self.catalog)
        self.assertFalse(result["required"])
        self.assertEqual("not-required",result["include"][0]["id"])

    def test_forged_selection_denied(self):
        with self.assertRaises(ValueError):
            module.select({"selected_gates":["atomic/docker/not-a-gate"]},self.catalog)


if __name__=="__main__":
    unittest.main()

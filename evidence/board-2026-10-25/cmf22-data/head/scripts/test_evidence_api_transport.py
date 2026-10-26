import json
import subprocess
import unittest
from unittest.mock import patch

from evidence_api_transport import GitHub


class TransportTests(unittest.TestCase):
    def setUp(self):
        self.api = GitHub('mcpdev80/baseharbor')

    def test_retries_only_failed_page_without_restarting_successful_pages(self):
        first = {'total_count': 11, 'jobs': [{'id': i} for i in range(1, 11)]}
        last = {'total_count': 11, 'jobs': [{'id': 11}]}
        error = subprocess.CalledProcessError(1, ['gh'], stderr=b'gh: Server Error (HTTP 502)')
        with patch('evidence_api_transport.subprocess.check_output', side_effect=[
                json.dumps(first).encode(), error, json.dumps(last).encode()]) as call, \
                patch('evidence_api_transport.time.sleep') as sleep:
            self.assertEqual(len(self.api.pages('actions/runs/1/jobs?filter=latest', 'jobs')), 11)
        self.assertEqual(call.call_count, 3)
        self.assertEqual(call.call_args_list[1].args, call.call_args_list[2].args)
        self.assertNotEqual(call.call_args_list[0].args, call.call_args_list[1].args)
        sleep.assert_called_once_with(2)

    def test_retry_budget_and_timeouts_are_bounded(self):
        for error in [subprocess.CalledProcessError(1, ['gh'], stderr=b'HTTP 503'),
                      subprocess.TimeoutExpired(['gh'], 60)]:
            with self.subTest(error=type(error).__name__), \
                    patch('evidence_api_transport.subprocess.check_output', side_effect=error) as call, \
                    patch('evidence_api_transport.time.sleep') as sleep:
                with self.assertRaises(type(error)):
                    self.api.get('actions/runs/1')
                self.assertEqual(call.call_count, 4)
                self.assertEqual([c.args[0] for c in sleep.call_args_list], [2, 4, 8])

    def test_auth_and_missing_evidence_fail_immediately(self):
        for status in [401, 403, 404, 422]:
            with self.subTest(status=status), \
                    patch('evidence_api_transport.subprocess.check_output', side_effect=
                          subprocess.CalledProcessError(1, ['gh'], stderr=f'HTTP {status}'.encode())) as call:
                with self.assertRaises(subprocess.CalledProcessError):
                    self.api.get('actions/runs/1')
                self.assertEqual(call.call_count, 1)

    def test_each_attempt_keeps_original_jobs_and_failures(self):
        jobs = [{'id': 10 + a, 'run_id': 1, 'run_attempt': a, 'conclusion': result}
                for a, result in [(1, 'failure'), (2, 'success')]]
        with patch.object(self.api, 'get', return_value={'id': 1, 'run_attempt': 2}), \
                patch.object(self.api, 'page_rows', side_effect=[[jobs[0]], [jobs[1]]]) as pages:
            self.assertEqual(self.api.pages('actions/runs/1/jobs?filter=all&per_page=30', 'jobs'), jobs)
            self.assertEqual([c.args[0] for c in pages.call_args_list],
                             ['actions/runs/1/attempts/1/jobs', 'actions/runs/1/attempts/2/jobs'])

    def test_truncated_duplicate_and_changing_pages_are_rejected(self):
        invalid = [
            [{'total_count': 2, 'jobs': [{'id': 1}]}, {'total_count': 2, 'jobs': []}],
            [{'total_count': 2, 'jobs': [{'id': 1}]}, {'total_count': 2, 'jobs': [{'id': 1}]}],
            [{'total_count': 2, 'jobs': [{'id': 1}]}, {'total_count': 1, 'jobs': []}],
            [{'total_count': 0, 'jobs': [{'id': 1}]}],
        ]
        for responses in invalid:
            with self.subTest(responses=responses), patch.object(self.api, 'get', side_effect=responses):
                with self.assertRaises(ValueError):
                    self.api.page_rows('actions/runs/1/jobs', 'jobs')

    def test_wrong_attempt_or_run_cannot_be_relabelled(self):
        for job in [{'id': 1, 'run_id': 2, 'run_attempt': 1},
                    {'id': 1, 'run_id': 1, 'run_attempt': 2}]:
            with patch.object(self.api, 'get', return_value={'id': 1, 'run_attempt': 1}), \
                    patch.object(self.api, 'page_rows', return_value=[job]):
                with self.assertRaises(ValueError):
                    self.api.pages('actions/runs/1/jobs?filter=all&per_page=30', 'jobs')


if __name__ == '__main__':
    unittest.main()

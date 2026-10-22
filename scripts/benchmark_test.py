import json
from pathlib import Path
import unittest

from benchmark import diagnostics, fixture_scores


class BenchmarkEvidenceTest(unittest.TestCase):
    def test_tool_failures_are_not_missed_bugs_and_scope_is_preserved(self):
        analysis = {'name': 'effectgo', 'format': 'analysis', 'exit_codes': [0, 3]}
        payload = json.dumps({'pkg': {'effects': [{'posn': '/cases/pure/bad/bad.go:7:6', 'message': 'impure'}]}})
        finding = diagnostics(analysis, payload, '', 3)
        self.assertEqual((finding[0]['rule'], finding[0]['line']), ('effects', 7))
        score = fixture_scores(finding, {'pure': {'effectgo': ['effects']}, 'nil': {}}, 'effectgo', Path('/cases'))
        self.assertEqual((score['detected'], score['applicable'], score['controls_with_findings']), (1, 1, 0))
        self.assertFalse(score['cases'][1]['applicable'])
        staticcheck = {'name': 'staticcheck', 'format': 'analysis', 'exit_codes': [0, 1]}
        ndjson = json.dumps({'code': 'SA5009', 'location': {'file': 'bad.go', 'line': 6}, 'message': 'format mismatch'})
        self.assertEqual(diagnostics(staticcheck, ndjson + '\n' + ndjson, '', 1)[0]['rule'], 'SA5009')
        vet = {'name': 'go-vet', 'format': 'analysis', 'exit_codes': [0, 1]}
        self.assertEqual(diagnostics(vet, '', '# pkg\n' + payload, 1)[0]['line'], 7)
        text = {'name': 'exhaustive', 'format': 'text', 'exit_codes': [0, 3]}
        self.assertEqual(diagnostics(text, '', 'bad.go:18:2: missing cases\n', 3)[0]['rule'], 'exhaustive')
        for stdout, stderr, code in [('', 'load failed', 2), ('{}', '', 3), ('not JSON', '', 0), ('{"pkg":{"error":"load failed"}}', '', 0)]:
            with self.assertRaises(ValueError):
                diagnostics(analysis, stdout, stderr, code)
        with self.assertRaises(ValueError):
            diagnostics(text, '', 'package load failed', 3)


if __name__ == '__main__':
    unittest.main()

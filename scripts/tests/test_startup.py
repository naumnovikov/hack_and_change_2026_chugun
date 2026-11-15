"""Exercise Bash startup decisions without starting Docker or downloading models."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


FAKE_DOCKER = '''#!/usr/local/bin/python
import os,sys,json
args=sys.argv[1:]
with open(os.environ['CALLS'],'a') as f:
    f.write(json.dumps({'args':args,'token_length':len(os.getenv('AI_INTERNAL_TOKEN',''))})+'\\n')
if 'python' in args and '-c' in args:
    print(os.environ['TOKEN_LENGTH']);print(os.environ['LOCAL']);print('custom:1');print('embed')
elif args[-2:]==['ollama','list']:
    print('NAME ID SIZE');print('custom:1 x 1GB');print('embed:latest x 1GB')
'''


class StartupTests(unittest.TestCase):
    def run_startup(self, local, token_length='0'):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'scripts').mkdir()
            (root / 'bin').mkdir()
            source = Path(__file__).resolve().parents[1] / 'start.sh'
            shutil.copyfile(source, root / 'scripts/start.sh')
            (root / '.env').touch()
            fake = root / 'bin/docker'
            fake.write_text(FAKE_DOCKER)
            fake.chmod(0o755)
            calls = root / 'calls.jsonl'
            env = dict(os.environ, PATH=str(root / 'bin') + ':' + os.environ['PATH'],
                       CALLS=str(calls), LOCAL=str(local), TOKEN_LENGTH=token_length)
            env.pop('AI_INTERNAL_TOKEN', None)
            result = subprocess.run(['bash', str(root / 'scripts/start.sh')], env=env,
                                    capture_output=True, text=True)
            import json
            return result, [json.loads(line) for line in calls.read_text().splitlines()]

    def test_remote_provider_skips_ollama_and_shares_ephemeral_token(self):
        result, calls = self.run_startup(0)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(any('ollama' in call['args'] for call in calls))
        final = calls[-2]
        self.assertIn('--no-deps', final['args'])
        self.assertEqual(final['token_length'], 64)

    def test_local_provider_uses_configured_cached_model_names(self):
        result, calls = self.run_startup(1)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(any('pull' in call['args'] for call in calls))
        self.assertTrue(any('ollama' in call['args'] for call in calls))

    def test_short_explicit_token_fails_before_starting_services(self):
        result, calls = self.run_startup(0, '8')
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(any('up' in call['args'] for call in calls))


if __name__ == '__main__':
    unittest.main()

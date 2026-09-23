#!/usr/bin/env python3
import json
import os
from pathlib import Path
import subprocess

root = Path(__file__).resolve().parents[1]
env = dict(os.environ, AI_MODE='mock')
env.pop('OPENAI_API_KEY', None)
def run(*args):
    completed = subprocess.run([str(root / 'bin/apm'), *args, '--json'], cwd=root, env=env, text=True, capture_output=True, check=True)
    return json.loads(completed.stdout)

names = run('scenario', 'list')
assert set(names) == {'queue-delay', 'receiver-failure', 'insufficient-evidence', 'evolving-evidence'}
initial = run('replay', 'queue-delay')
incident = initial['incident']['id']
proposal = initial['proposal']['id']
assert initial['incident']['status'] == 'awaiting_review'
assert run('proposal', 'show', proposal)['status'] == 'pending'
original_env = env.copy()
env.update(AI_MODE='invalid', OPENAI_TIMEOUT='invalid', MAX_INVESTIGATION_STEPS='invalid')
assert run('incident', 'show', incident)['incident']['id'] == incident
assert run('proposal', 'show', proposal)['status'] == 'pending'
assert isinstance(run('incident', 'timeline', incident), list)
subprocess.run([str(root / 'bin/apm'), 'migrate'], cwd=root, env=env, capture_output=True, text=True, check=True)
env.clear()
env.update(original_env)

assert run('proposal', 'approve', proposal, '--note', 'CLI smoke reviewer')['status'] == 'approved'
partial = run('replay', 'queue-delay', '--incident', incident)
assert partial['recovery']['unresolved_original_items'] == 1
assert not partial['recovery']['closure_allowed']
full = run('replay', 'queue-delay', '--incident', incident)
assert full['recovery']['closure_allowed']
assert full['incident']['status'] == 'closure_proposed'
closure = full['proposal']['id']
assert run('proposal', 'reject', closure, '--note', 'Request repeat check')['status'] == 'rejected'
full = run('replay', 'queue-delay', '--incident', incident)
assert full['proposal']['id'] != closure
run('proposal', 'approve', full['proposal']['id'], '--note', 'Cohort verified')
assert run('incident', 'show', incident)['incident']['status'] == 'closed'
timeline = run('incident', 'timeline', incident)
assert any(e['kind'] == 'recovery_evidence' for e in timeline)
assert sum(e['kind'] == 'proposal_approved' for e in timeline) == 2
evolving = run('replay', 'evolving-evidence')
continued = run('replay', 'evolving-evidence', '--incident', evolving['incident']['id'], '--next')
assert continued['proposal']['owner'] == 'merchant_integration'
assert continued['incident']['revision'] > evolving['incident']['revision']
stale = subprocess.run([str(root / 'bin/apm'), 'proposal', 'approve', evolving['proposal']['id']], cwd=root, env=env, capture_output=True, text=True)
assert stale.returncode != 0 and 'stale' in stale.stderr
manual = run('replay', 'insufficient-evidence')
retried = run('replay', 'insufficient-evidence', '--incident', manual['incident']['id'], '--retry')
assert retried['incident']['id'] == manual['incident']['id'] and retried['incident']['clock'] == manual['incident']['clock']
assert retried['incident']['revision'] > manual['incident']['revision']

live = subprocess.run([str(root / 'bin/apm'), 'replay', 'queue-delay', '--ai-mode', 'live'], cwd=root, env=env, capture_output=True, text=True)
assert live.returncode != 0 and 'OPENAI_API_KEY' in live.stderr
print('CLI SMOKE PASSED')

# Follow the actual printed commands, including paths with spaces, without a shell.
import shlex
import tempfile

def text_command(argv):
    result = subprocess.run(argv, cwd=root, env=env, text=True, capture_output=True, check=True)
    return result.stdout

def printed(text, token='Next: '):
    lines = [line for line in text.splitlines() if line.startswith(token)]
    assert len(lines) == 1, text
    return shlex.split(lines[0][len(token):])

with tempfile.TemporaryDirectory(prefix='cli context ', dir=root / '.tools') as directory:
    directory = Path(directory)
    executable = directory / 'apm executable'
    executable.symlink_to(root / 'bin/apm')
    fixtures = directory / 'scenario fixtures'
    fixtures.symlink_to(root / 'testdata/scenarios', target_is_directory=True)
    output = text_command([str(executable), '--scenarios-dir', str(fixtures), '--without-memory', 'replay', 'queue-delay', '--ai-mode', 'mock'])
    def assert_context(argv):
        assert argv[0] == str(executable), argv
        assert argv[argv.index('--ai-mode') + 1] == 'mock', argv
        assert argv[argv.index('--scenarios-dir') + 1] == str(fixtures), argv
        assert '--without-memory' in argv, argv
    for verb in ['reject', 'approve']:
        view = printed(output)
        assert_context(view)
        env['AI_MODE'] = 'invalid'
        output = text_command(view)
        choices = [shlex.split(line) for line in output.splitlines() if f' proposal {verb} ' in line]
        assert len(choices) == 1, output
        assert_context(choices[0])
        output = text_command(choices[0])
        continuation = printed(output)
        assert_context(continuation)
        output = text_command(continuation)
    # After escalation approval: partial recovery, full recovery, closure approval.
    output = text_command(printed(output))
    output = text_command(printed(output))
    approval = [shlex.split(line) for line in output.splitlines() if ' proposal approve ' in line]
    assert len(approval) == 1, output
    output = text_command(approval[0])
    assert 'Incident closed' in output and 'Next:' not in output, output
print('GUIDED CLI CONTEXT PASSED')

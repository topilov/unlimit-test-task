#!/usr/bin/env python3
import json
import os
from pathlib import Path
import subprocess

root = Path(__file__).resolve().parents[1]
env = dict(os.environ, AI_MODE='mock')
env.pop('OPENAI_API_KEY', None)

def run(*args):
    result = subprocess.run([str(root / 'bin/apm'), *args, '--json'], cwd=root, env=env, text=True, capture_output=True)
    if result.returncode:
        raise RuntimeError(result.stderr or result.stdout)
    return json.loads(result.stdout)

def investigate(*flags):
    out = run('replay', 'queue-delay', '--ai-mode', 'mock', *flags)
    return run('incident', 'show', out['incident']['id'])['runs'][-1]

source = investigate('--without-memory')
original_trace = run('incident', 'timeline', source['incident_id'])
result = run('feedback', 'add', source['id'], '--verdict', 'helpful', '--note', 'Checking dispatch before blaming the receiver was useful.')
lesson = result['lesson']
assert lesson['status'] == 'pending'
assert run('feedback', 'reflect', result['feedback']['id'])['lesson']['id'] == lesson['id']
assert all(m['id'] != lesson['id'] for m in investigate()['memory'])
try:
    assert run('memory', 'approve', lesson['id'])['status'] == 'active'
    later = investigate()
    assert any(m['id'] == lesson['id'] for m in later['memory'])
    assert not investigate('--without-memory')['memory']
finally:
    run('memory', 'disable', lesson['id'])
assert all(m['id'] != lesson['id'] for m in investigate()['memory'])
history = run('incident', 'show', later['incident_id'])['runs'][-1]
assert any(m['id'] == lesson['id'] for m in history['memory'])
assert run('incident', 'show', source['incident_id'])['runs'][-1] == source
assert run('incident', 'timeline', source['incident_id'])[:len(original_trace)] == original_trace
print('Feedback -> reflection -> pending lesson: PASS')
print('Approval -> later use -> disable: PASS')
print('Baseline and immutable history: PASS')
print('MOCK FEEDBACK LOOP PASSED (scripted mechanics; live improvement is unverified)')

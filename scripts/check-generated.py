#!/usr/bin/env python3
from pathlib import Path
import subprocess
import sys

root = Path(__file__).resolve().parents[1]
generated = root / 'internal/db/generated'
def snapshot():
    return {p.name: p.read_bytes() for p in generated.glob('*.go')}
before = snapshot()
sqlc = root / '.tools/bin/sqlc'
subprocess.run([str(sqlc) if sqlc.exists() else 'sqlc', 'generate'], cwd=root, check=True)
after = snapshot()
if before != after:
    sys.exit('Generated files changed. Run make generate and review the diff.')
print('GENERATED CODE MATCHES')

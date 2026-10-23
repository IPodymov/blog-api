"""Additional contract checks; run together with Redocly OpenAPI 3.1 validation."""
import json
import re
import subprocess
import sys
from pathlib import Path

spec = json.loads(Path('api/openapi.yaml').read_text())
assert spec['openapi'] == '3.1.0'
ids = []
for path, methods in spec['paths'].items():
    for method, op in methods.items():
        ids.append(op['operationId'])
        assert 'security' in op and op['responses'] and op['tags']
        names = {p['name'] for p in op['parameters'] if '$ref' not in p and p['in'] == 'path'}
        assert names == set(re.findall(r'{([^}]+)}', path)), path
assert len(ids) == len(set(ids)), 'duplicate operationId'


def walk(v):
    if isinstance(v, dict):
        if '$ref' in v:
            target = spec
            assert v['$ref'].startswith('#/'), 'only local refs expected'
            for part in v['$ref'][2:].split('/'): target = target[part.replace('~1', '/').replace('~0', '~')]
        for x in v.values(): walk(x)
    elif isinstance(v, list):
        for x in v: walk(x)


walk(spec)
source = Path('internal/httpapi/api.go').read_text()
routes = {(method.lower(), path if path in ['/health', '/ready', '/docs', '/openapi.yaml'] else '/api/v1' + path) for
          method, path in re.findall(r'\br\.(Get|Post|Patch|Put|Delete)\("([^"]+)"', source)}
documented = {(method, path) for path, ops in spec['paths'].items() for method in ops}
assert routes == documented, f'Route mismatch: {routes ^ documented}'
before = Path('api/openapi.yaml').read_bytes()
subprocess.run([sys.executable, 'scripts/generate_openapi.py'], check=True)
assert Path('api/openapi.yaml').read_bytes() == before, 'generated specification is stale'
print(f'Contract checked: {len(ids)} operations, local refs resolved, routes match, generation deterministic.')

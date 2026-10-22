#!/usr/bin/env python3
"""Compare pinned Go analyzers on paired fixtures and one public source snapshot."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import statistics
import subprocess
import time

ROOT = Path(__file__).resolve().parents[1]
POSITION = re.compile(r"^(.+\.go):(\d+)(?::\d+)?(?::\s*(.*))?$")


def diagnostics(tool, stdout, stderr, code):
    if code not in tool['exit_codes']:
        raise ValueError(f"{tool['name']}: tool failure ({code}): {stderr}")
    found = []
    text = stdout + '\n' + stderr
    if tool['format'] == 'text':
        for line in text.splitlines():
            if not line.strip():
                continue
            match = POSITION.match(line)
            if not match:
                raise ValueError(f"unexpected {tool['name']} output: {line}")
            found.append({'file': match[1], 'line': int(match[2]), 'rule': tool['name'], 'message': match[3] or ''})
    else:
        text = '\n'.join(line for line in text.splitlines() if not line.startswith(('# ', 'effectgo: ')))
        decoder = json.JSONDecoder()
        def visit(value, rule=''):
            if isinstance(value, list):
                for item in value:
                    visit(item, rule)
            elif isinstance(value, dict):
                if 'error' in value:
                    raise ValueError(f"analysis error: {value['error']}")
                if 'posn' in value:
                    match = POSITION.match(value['posn'])
                    if not match:
                        raise ValueError(f"invalid diagnostic position: {value['posn']}")
                    found.append({'file': match[1], 'line': int(match[2]), 'rule': rule, 'message': value['message']})
                elif 'location' in value and 'code' in value:
                    found.append({'file': value['location']['file'], 'line': value['location']['line'], 'rule': value['code'], 'message': value['message']})
                else:
                    for key, item in value.items():
                        visit(item, key)
        while text.strip():
            value, end = decoder.raw_decode(text.lstrip())
            text = text.lstrip()[end:]
            visit(value)
    if code and not found:
        raise ValueError(f"{tool['name']}: nonzero exit without parsed findings: {text}")
    return sorted(found, key=lambda d: (d['file'], d['line'], d['rule'], d['message']))


def fixture_scores(findings, rules, name, corpus):
    matched = set()
    rows = []
    for case, tools in rules.items():
        expected = tools.get(name)
        hits = {}
        for variant in ['bad', 'good']:
            prefix = str(corpus / case / variant) + '/'
            selected = [i for i, item in enumerate(findings) if item['file'].startswith(prefix) and item['rule'] in (expected or [])]
            matched.update(selected)
            hits[variant] = bool(selected)
        rows.append({'case': case, 'applicable': expected is not None, **hits})
    return {'cases': rows, 'detected': sum(r['bad'] for r in rows if r['applicable']),
            'applicable': sum(r['applicable'] for r in rows),
            'controls_with_findings': sum(r['good'] for r in rows if r['applicable']),
            'other_findings': [item for i, item in enumerate(findings) if i not in matched]}


def checked(args, env, cwd, log):
    result = subprocess.run(args, cwd=cwd, env=env, text=True, capture_output=True, timeout=600)
    log.write_text(result.stdout + result.stderr)
    if result.returncode:
        raise RuntimeError(f"command failed ({result.returncode}): {args}; see {log}")
    return result.stdout


def measure(tool, command, cwd, env, directory, label):
    usage = directory / (label + '.usage')
    started = time.perf_counter()
    result = subprocess.run(['/usr/bin/time', '-f', '%U %S %M', '-o', str(usage), *command], cwd=cwd, env=env, text=True, capture_output=True, timeout=600)
    elapsed = time.perf_counter() - started
    (directory / (label + '.stdout')).write_text(result.stdout)
    (directory / (label + '.stderr')).write_text(result.stderr)
    cpu_user, cpu_sys, rss = usage.read_text().strip().splitlines()[-1].split()
    findings = diagnostics(tool, result.stdout, result.stderr, result.returncode)
    for item in findings:
        item['file'] = str((cwd / item['file']).resolve())
    return {'seconds': elapsed, 'user_seconds': float(cpu_user), 'sys_seconds': float(cpu_sys),
            'max_rss_kib': int(rss), 'exit_code': result.returncode, 'findings': findings}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True, help='new directory; never overwritten')
    parser.add_argument('--samples', type=int, default=5)
    args = parser.parse_args()
    if args.samples < 3:
        parser.error('--samples must be at least 3')
    out = args.output.resolve()
    if out.exists():
        parser.error('--output must not already exist')
    config = json.loads((ROOT / 'benchmarks/toolchain.json').read_text())
    env = dict(os.environ, GOWORK='off', GOTOOLCHAIN='local', GOFLAGS='-buildvcs=false -p=1',
               GOMAXPROCS='2', GOMEMLIMIT='2GiB', LC_ALL='C', GOPROXY='https://proxy.golang.org',
               GOPRIVATE='', GONOPROXY='', GONOSUMDB='', GOSUMDB='sum.golang.org', GOAUTH='off')
    for key in ['GOROOT', 'GOCACHEPROG', 'STATICCHECK_CACHE']:
        env.pop(key, None)
    if not Path('/usr/bin/time').is_file():
        parser.error('GNU /usr/bin/time is required')
    version = subprocess.check_output(['go', 'env', 'GOVERSION'], env=env, text=True).strip()
    if version != config['go_version']:
        parser.error(f"need {config['go_version']}; found {version}")
    out.mkdir(parents=True)
    env.update(GOMODCACHE=str(out / 'modules'), GOBIN=str(out / 'bin'), GOCACHE=str(out / 'setup-cache'))
    tools = config['tools']
    commands = {}
    for tool in tools:
        directory = out / tool['name']
        directory.mkdir()
        if tool.get('install'):
            checked(['go', 'install', '-trimpath', tool['install']], env, ROOT, directory / 'install.log')
            binary = out / 'bin' / tool['name']
            checked(['go', 'version', '-m', str(binary)], env, ROOT, directory / 'build-info.log')
            tool['binary_sha256'] = hashlib.sha256(binary.read_bytes()).hexdigest()
            commands[tool['name']] = [str(binary), *tool['args']]
        else:
            commands[tool['name']] = ['go', *tool['args']]
    corpus = ROOT / 'benchmarks/corpus'
    checked(['go', 'build', './...'], env, corpus, out / 'corpus-build.log')
    info = json.loads(checked(['go', 'mod', 'download', '-json', config['public_module']], env, out, out / 'public-download.json'))
    target = out / 'public-source'
    shutil.copytree(info['Dir'], target, ignore=shutil.ignore_patterns('*_test.go', '.git'))
    checked(['go', 'build', './...'], env, target, out / 'public-build.log')
    source_files = {str(p.relative_to(target)): hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(target.rglob('*.go'))}
    result = {'go_version': version, 'platform': platform.platform(), 'machine': platform.machine(),
              'cpu_count': os.cpu_count(), 'gomaxprocs': 2, 'samples': args.samples,
              'public_module': info, 'public_source_go_files': source_files, 'tests_excluded': True,
              'method': 'fresh GOCACHE/XDG per tool; one cold run, one warmup, rotated warm rounds; no filesystem cache flush',
              'tools': {}}
    for tool in tools:
        name = tool['name']
        directory = out / name
        tool_env = dict(env, XDG_CACHE_HOME=str(directory / 'accuracy-xdg'))
        accuracy = measure(tool, commands[name], corpus, tool_env, directory, 'accuracy')
        cache = directory / 'cache'
        tool_env = dict(env, GOCACHE=str(cache / 'go'), XDG_CACHE_HOME=str(cache / 'xdg'))
        cold = measure(tool, commands[name], target, tool_env, directory, 'cold')
        warmup = measure(tool, commands[name], target, tool_env, directory, 'warmup')
        if cold['findings'] != warmup['findings']:
            raise RuntimeError(f'{name}: cold/warm findings differ')
        result['tools'][name] = {'configuration': tool, 'command': commands[name], 'accuracy': accuracy,
                                 'score': fixture_scores(accuracy['findings'], config['cases'], name, corpus),
                                 'cold': cold, 'warm': []}
        (out / 'results.json').write_text(json.dumps(result, indent=2) + '\n')
        print(f"{name}: accuracy {result['tools'][name]['score']['detected']}/{result['tools'][name]['score']['applicable']}; cold {cold['seconds']:.3f}s", flush=True)
    for round_index in range(args.samples):
        for tool in tools[round_index % len(tools):] + tools[:round_index % len(tools)]:
            name = tool['name']
            directory = out / name
            tool_env = dict(env, GOCACHE=str(directory / 'cache/go'), XDG_CACHE_HOME=str(directory / 'cache/xdg'))
            sample = measure(tool, commands[name], target, tool_env, directory, f'warm-{round_index + 1}')
            if sample['findings'] != result['tools'][name]['cold']['findings']:
                raise RuntimeError(f'{name}: repeated findings differ')
            result['tools'][name]['warm'].append(sample)
    rows = ['| Tool | Applicable bad cases detected | Controls with findings | Fresh cache (s) | Warm median (s) | Warm range (s) | Warm max RSS (MiB) |', '|---|---:|---:|---:|---:|---:|---:|']
    for name, data in result['tools'].items():
        warm = data['warm']
        values = [r['seconds'] for r in warm]
        data['warm_median_seconds'] = statistics.median(values)
        score = data['score']
        rows.append(f"| {name} | {score['detected']}/{score['applicable']} | {score['controls_with_findings']} | {data['cold']['seconds']:.3f} | {statistics.median(values):.3f} | {min(values):.3f}–{max(values):.3f} | {max(r['max_rss_kib'] for r in warm) / 1024:.1f} |")
    result['status'] = 'complete'
    (out / 'results.json').write_text(json.dumps(result, indent=2) + '\n')
    (out / 'report.md').write_text('\n'.join(rows) + '\n')
    print('\n'.join(rows))


if __name__ == '__main__':
    main()

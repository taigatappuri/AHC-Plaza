#!/usr/bin/env python3
"""Exercise the release executable with real C++/pahcer and no Python on PATH."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import sqlite3
import subprocess
import tempfile
import time

def fixture(root):
    (root / 'solver').mkdir()
    for name, values in [('train', [3, 4])]:
        directory = root / 'ahc-plaza/inputs' / name
        directory.mkdir(parents=True)
        for index, value in enumerate(values):
            (directory / f'{index}.txt').write_text(f'{value}\n')
    (root / 'solver/main.cpp').write_text('''#include <cstdio>
#include <unistd.h>
constexpr int WIDTH = 1; // @tune 1 10
constexpr int BONUS = 0; // @tune -2 2
int main(){int x; if(scanf("%d", &x)!=1)return 2; usleep(30000); printf("%d\\n", WIDTH); fprintf(stderr,"Score = %d\\n",100-(WIDTH-x)*(WIDTH-x)+BONUS);}
''')
    (root / 'ahc-plaza.toml').write_text('''[project]
problem="test"
objective="max"
[paths]
solver_dir="solver"
tools_dir="tools"
[execution]
default_input_dir="ahc-plaza/inputs"
threads=1
timeout_ms=1000
[pahcer]
setting_file="pahcer_config.toml"
''')
    (root / 'pahcer_config.toml').write_text('''[general]
version = "0.3.1"
[problem]
problem_name = "test"
objective = "Max"
score_regex = 'Score = (?P<score>\\d+)'
[test]
start_seed = 0
end_seed = 2
threads = 1
out_dir = "./pahcer"
[[test.compile_steps]]
program = "g++"
args = ["-std=c++17", "-O2", "main.cpp", "-o", "main.exe"]
[[test.test_steps]]
program = "./main.exe"
args = []
stdin = "./tools/in/{SEED04}.txt"
stdout = "./tools/out/{SEED04}.txt"
stderr = "./tools/err/{SEED04}.txt"
measure_time = true
''')

def assert_tuning_targets_removed(root, study_id):
    with sqlite3.connect(root / 'ahc-plaza/ahc-plaza.db') as db:
        study = json.loads(db.execute('SELECT data FROM tuning_studies WHERE id=?', (study_id,)).fetchone()[0])
        run_ids = [study['baseline_run']]
        run_ids.extend(row[0] for row in db.execute('SELECT run_id FROM tuning_trials WHERE study_id=?', (study_id,)))
    remaining = [run_id for run_id in run_ids if (root / 'ahc-plaza/runs' / run_id / 'workspace/tools/target').exists()]
    assert not remaining, f'tuning target caches remain: {remaining}'
    copied_sources = [run_id for run_id in run_ids if (root / 'ahc-plaza/runs' / run_id / 'workspace/main.cpp').exists()]
    assert not copied_sources, f'tuning project copies remain: {copied_sources}'

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--binary', required=True)
    parser.add_argument('--trials', type=int, default=3)
    args = parser.parse_args()
    binary = str(Path(args.binary).resolve())
    with tempfile.TemporaryDirectory(prefix='plaza-smoke-') as temporary:
        temporary_root = Path(temporary)
        root = temporary_root / 'project'
        root.mkdir()
        fixture(root)
        path = temporary_root / 'path'
        path.mkdir()
        for command in ['sh', 'g++', 'as', 'ld', 'pahcer']:
            resolved = shutil.which(command)
            if not resolved:
                raise RuntimeError(f'{command} is required for this integration test')
            (path / command).symlink_to(resolved)
        environment = {**os.environ, 'PATH': str(path), 'HTTP_PROXY':'http://127.0.0.1:1', 'HTTPS_PROXY':'http://127.0.0.1:1', 'ALL_PROXY':'http://127.0.0.1:1', 'NO_PROXY':''}
        def run(*command, success=True):
            result = subprocess.run([binary, *command], cwd=root, env=environment, capture_output=True, text=True, timeout=240)
            if success and result.returncode:
                raise AssertionError(result.stdout + result.stderr)
            return result
        run('doctor')
        assert not (root / 'ahc-plaza/runtime').exists()
        ordinary = json.loads(run('run', '--solver', 'solver/main.cpp', '--input-dir', 'ahc-plaza/inputs/train', '--threads', '1', '--json').stdout)
        assert (root / 'ahc-plaza/runs' / ordinary['run_id'] / 'workspace/main.cpp').exists()
        assert not (root / 'ahc-plaza/runtime').exists()
        original = (root / 'solver/main.cpp').read_bytes()
        print(f'Running {args.trials} real candidates...', flush=True)
        result = json.loads(run('tune', '--solver', 'solver/main.cpp', '--input-dir', 'ahc-plaza/inputs/train', '--trials', str(args.trials), '--threads', '1').stdout)
        assert result['status'] == 'completed' and result['completed'] == args.trials, result
        identifier = result['id']
        assert_tuning_targets_removed(root, identifier)
        run('doctor', '--tuning')
        exported = json.loads(run('tune', 'export', '--study', identifier, '--best').stdout)
        run('run', '--solver', exported['path'], '--input-dir', 'ahc-plaza/inputs/train', '--threads', '1', '--json')
        assert original == (root / 'solver/main.cpp').read_bytes()
        # Source/config/input edits must not enter subsequent trials of this Study.
        (root / 'solver/main.cpp').write_text('this is deliberately not C++')
        (root / 'ahc-plaza/inputs/train/0.txt').write_text('9999')
        resumed = json.loads(run('tune', 'resume', '--study', identifier, '--additional-trials', '2').stdout)
        assert resumed['completed'] == args.trials + 2, resumed
        assert_tuning_targets_removed(root, identifier)
        # Graceful interruption and an ungraceful kill both preserve a resumable Study.
        for mode in ['terminate', 'kill']:
            process = subprocess.Popen([binary, 'tune', 'resume', '--study', identifier, '--additional-trials', '5'], cwd=root, env=environment, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            deadline = time.monotonic() + 40
            db_path = root / 'ahc-plaza/ahc-plaza.db'
            seen = False
            while time.monotonic() < deadline and process.poll() is None:
                with sqlite3.connect(db_path) as db:
                    data = db.execute("SELECT data FROM tuning_trials WHERE study_id=? ORDER BY number DESC LIMIT 1", (identifier,)).fetchone()
                    if data and json.loads(data[0])['status'] == 'RUNNING':
                        seen = True
                        break
                time.sleep(.025)
            assert seen, process.communicate(timeout=5)
            getattr(process, mode)()
            process.communicate(timeout=15)
            resumed = json.loads(run('tune', 'resume', '--study', identifier).stdout)
            assert resumed['status'] == 'completed', resumed
            assert resumed['completed'] + resumed['failed'] == resumed['trials'], resumed
            assert_tuning_targets_removed(root, identifier)
        with sqlite3.connect(root / 'ahc-plaza/ahc-plaza.db') as db:
            trials = [json.loads(row[0]) for row in db.execute('SELECT data FROM tuning_trials WHERE study_id=?', (identifier,))]
            assert len({t['run_id'] for t in trials}) == len(trials)
            assert all(t['delivered'] for t in trials)
            assert db.execute('SELECT count(*) FROM tuning_outbox WHERE delivered=0').fetchone()[0] == 0
        with sqlite3.connect(root / 'ahc-plaza/tuning' / identifier / 'optuna.sqlite3') as db:
            assert db.execute("SELECT count(*) FROM trials WHERE state='RUNNING'").fetchone()[0] == 0
        fixed = root / 'ahc-plaza/tuning' / identifier / 'fixed/inputs/0.txt'
        fixed.write_text('tampered')
        assert run('tune', 'resume', '--study', identifier, success=False).returncode != 0
        # Minimize: a failed candidate must never win through invalid_score=0.
        (root / 'ahc-plaza.toml').write_text((root / 'ahc-plaza.toml').read_text().replace('objective="max"', 'objective="min"'))
        (root / 'pahcer_config.toml').write_text((root / 'pahcer_config.toml').read_text().replace('objective = "Max"', 'objective = "Min"'))
        (root / 'ahc-plaza/inputs/train/0.txt').write_text('3')
        (root / 'solver/main.cpp').write_text('#include <cstdio>\nconstexpr int WIDTH=0; // @tune 1 2\nstatic_assert(WIDTH==0);\nint main(){fprintf(stderr,"Score = 10\\n");}\n')
        failed_result = run('tune', '--solver', 'solver/main.cpp', '--input-dir', 'ahc-plaza/inputs/train', '--trials', '10', '--threads', '1', success=False)
        failed = json.loads(failed_result.stdout)
        assert failed_result.returncode != 0 and failed['status'] == 'failed' and failed['failed'] == 5, failed
        assert failed['best_run'] == failed['baseline_run'] and failed['best_value'] == 10, failed
        run('tune', 'clean-runtime')
        assert not (root / 'ahc-plaza/runtime').exists()
        assert (root / 'ahc-plaza/tuning' / identifier / 'manifest.json').exists()
        print(json.dumps({'result':'PASS', 'candidate_count':len(trials), 'successful':resumed['completed'], 'failed_or_interrupted':resumed['failed'], 'checks':['ordinary Run without runtime','offline bundled Python','real Optuna/C++/pahcer','export and rerun','frozen source/input','pause/resume','SIGKILL recovery','tuning target cleanup','outbox reconciliation','tamper rejection','runtime cleanup']}, ensure_ascii=False))

if __name__ == '__main__':
    main()

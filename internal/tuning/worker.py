"""Versioned JSONL worker. stdout is reserved for protocol responses."""
import json
import math
import sys
import traceback

import optuna
from optuna.trial import TrialState

VERSION = 1
MAX_MESSAGE = 1 << 20
study = None
space = {}

def trial_data(trial):
    return dict(number=trial.number, params=trial.params, value=trial.value,
                status=trial.state.name, request_id=trial.user_attrs.get('request_id', ''),
                result_hash=trial.user_attrs.get('result_hash', ''))

def handle(req):
    global study, space
    operation = req['op']
    if operation == 'init':
        sampler = optuna.samplers.TPESampler(seed=req['seed'], n_startup_trials=10)
        study = optuna.create_study(storage='sqlite:///' + req['storage'],
                                   study_name=req['study_id'], direction=req['direction'],
                                   load_if_exists=True, sampler=sampler,
                                   pruner=optuna.pruners.NopPruner())
        previous = study.user_attrs.get('manifest_hash')
        if previous is not None and previous != req['manifest_hash']:
            raise ValueError('Study conditions differ from the saved manifest')
        study.set_user_attr('manifest_hash', req['manifest_hash'])
        space = {}
        for p in req['parameters']:
            if not p['enabled']:
                continue
            if p['type'] in ('int', 'long long'):
                space[p['name']] = optuna.distributions.IntDistribution(int(p['low']), int(p['high']), log=p['log'], step=int(p['step'] or 1))
            else:
                space[p['name']] = optuna.distributions.FloatDistribution(p['low'], p['high'], log=p['log'], step=p['step'])
        # An ask committed before its request attribute was written has no owner.
        for t in study.get_trials(deepcopy=False, states=(TrialState.RUNNING,)):
            if not t.user_attrs.get('request_id'):
                study.tell(t.number, state=TrialState.FAIL)
        return dict(worker_version=VERSION, python=sys.version.split()[0], optuna=optuna.__version__)
    if study is None:
        raise ValueError('Worker has not been initialized')
    if operation == 'ask':
        for t in study.get_trials(deepcopy=False):
            if t.user_attrs.get('request_id') == req['request_id']:
                return trial_data(t)
        trial = study.ask(fixed_distributions=space)
        trial.set_user_attr('request_id', req['request_id'])
        trial.set_user_attr('study_id', study.study_name)
        return dict(number=trial.number, params=trial.params, status='RUNNING', request_id=req['request_id'])
    if operation == 'tell':
        number = req['number']
        t = next(t for t in study.get_trials(deepcopy=False) if t.number == number)
        state = TrialState.COMPLETE if req['status'] == 'COMPLETE' else TrialState.FAIL
        value = req.get('value') if state == TrialState.COMPLETE else None
        if state == TrialState.COMPLETE and (value is None or not math.isfinite(value)):
            raise ValueError('Objective must be finite')
        if t.state.is_finished():
            if t.state != state or t.value != value or t.user_attrs.get('result_hash') != req['result_hash']:
                raise ValueError('Conflicting trial result; refusing overwrite')
            return trial_data(t)
        # Public Trial API records diagnostic metadata before the terminal transition.
        live = optuna.trial.Trial(study, t._trial_id)
        live.set_user_attr('result_hash', req['result_hash'])
        live.set_user_attr('reason', req.get('reason', ''))
        result = study.tell(number, values=value, state=state)
        return trial_data(result)
    if operation == 'list_trials':
        offset, limit = req.get('offset', 0), min(req.get('limit', 100), 100)
        return [trial_data(t) for t in study.get_trials(deepcopy=False)[offset:offset+limit]]
    if operation == 'get_trial':
        return trial_data(next(t for t in study.get_trials(deepcopy=False) if t.number == req['number']))
    if operation == 'best':
        return trial_data(study.best_trial)
    raise ValueError('Unknown operation')

for line in iter(lambda: sys.stdin.buffer.readline(MAX_MESSAGE + 1), b''):
    request = {}
    try:
        if len(line) > MAX_MESSAGE or not line.endswith(b'\n'):
            raise ValueError('Message exceeds limit')
        request = json.loads(line)
        if request.get('version') != VERSION:
            raise ValueError('Unsupported protocol version')
        response = dict(version=VERSION, id=request['id'], result=handle(request))
    except Exception as error:
        traceback.print_exc(file=sys.stderr)
        response = dict(version=VERSION, id=request.get('id'), error=str(error), error_type=type(error).__name__)
    sys.stdout.write(json.dumps(response, allow_nan=False, separators=(',', ':')) + '\n')
    sys.stdout.flush()

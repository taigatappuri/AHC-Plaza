<script lang="ts">
  import { onMount } from 'svelte'
  import { errorMessage, requestJSON } from '../lib/api'
  import type { TuneParameter, TuneScan, TuneStudy, TuneTrial, TuneEnvironment, TuneDetail } from '../lib/tuning'
  export let solvers: string[] = []
  export let inputDirectories: string[] = []
  export let objective = 'max'
  export let onOpenRun: (id: string) => void = () => {}
  export let onCompare: (a: string, b: string) => void = () => {}

  let solver = '', inputDir = '', trials = 100, threads = 0, timeoutMS = 0, seconds = 0, seed = 42
  let scan: TuneScan | null = null, parameters: TuneParameter[] = [], environment: TuneEnvironment | null = null
  let studies: TuneStudy[] = [], detail: TuneDetail | null = null, rows: TuneTrial[] = [], history: TuneTrial[] = []
  let message = '', busy = false, page = 0, additional = 0, validationInput = '', validationThreads = 1
  let events: EventSource | null = null, refreshing = false, lastRevision = '', destroyed = false
  let selectedTrial: number | null = null
  let selectedStudyID = ""
  let previousParameters: TuneParameter[] = [], caseCount: number | null = null, countInput = ""
  $: if (inputDir && inputDir !== countInput) { countInput = inputDir; const requested = inputDir; caseCount=null; void requestJSON<{count:number}>(`/api/tuning/input-count?input_dir=${encodeURIComponent(requested)}`).then(r => { if(inputDir===requested) caseCount=r.count }).catch(e=>message=errorMessage(e)) }
  $: if (!solver && solvers.length) solver = solvers[0]
  $: if (!inputDir && inputDirectories.length) inputDir = inputDirectories[0]
  $: incomplete = parameters.length === 0 || !parameters.some(p => p.enabled) || parameters.some(p => p.enabled && (p.low == null || p.high == null || !Number.isFinite(p.low) || !Number.isFinite(p.high) || p.low > p.high))
  $: baseline = detail?.study.baseline_value ?? null
  $: values = [...history.filter(t => t.value !== null && t.status === 'COMPLETE').map(t => t.value!), ...(baseline === null ? [] : [baseline])]
  $: lo = values.length ? Math.min(...values) : 0
  $: hi = values.length ? Math.max(...values) : 1
  $: chartCount = Math.max(1, ...history.map(t => t.number + 1))
  $: chartPoints = history.filter(t => t.value !== null && t.status === 'COMPLETE').map(t => ({ x: 35 + (t.number + 1) / chartCount * 710, y: 155 - ((t.value! - lo) / (hi - lo || 1)) * 130, trial: t }))
  $: bestLine = buildBestLine(history, baseline)
  $: active = detail?.active ?? false
  const states: Record<string, string> = { preparing: '準備・現在値を評価中', running: '候補を評価中', stopping: '停止処理中', paused: '一時停止', completed: '完了', failed: '失敗', COMPLETE: '成功', FAIL: '失敗', RUNNING: '評価中', cancelled: '中断' }
  const mb = (bytes: number) => `${(bytes / 1e6).toFixed(1)} MB`
  const number = (value: number | null | undefined) => value == null ? '—' : value.toLocaleString(undefined, { maximumSignificantDigits: 7 })
  function buildBestLine(items: TuneTrial[], base: number | null) {
    let best = base; const points: string[] = []; const count = Math.max(1,...items.map(t => t.number+1))
    const direction = detail?.manifest.prepared.config.File.project.objective ?? objective
    items.forEach(t => { if (t.status === 'COMPLETE' && t.value !== null) best = best === null ? t.value : direction === 'min' ? Math.min(best, t.value) : Math.max(best, t.value)
      if (best !== null) points.push(`${35 + (t.number + 1) / count * 710},${155 - (best - lo) / (hi - lo || 1) * 130}`)
    }); return points.join(' ')
  }
  async function post<T>(path: string, body: unknown = {}): Promise<T> { return requestJSON<T>(`/api/tuning/${path}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }) }
  async function action(fn: () => Promise<void>) { if (busy) return; busy = true; message = ''; try { await fn() } catch (e) { message = errorMessage(e) } finally { busy = false } }
  async function loadStudies() { studies = await requestJSON<TuneStudy[]>('/api/tuning/studies') }
  async function detect() { await action(async () => { const response = await post<{ scan: TuneScan; profile: TuneParameter[] | null; previous_profile: { parameters: TuneParameter[] } | null }>('scan', { solver }); scan = response.scan; previousParameters=response.previous_profile?.parameters ?? []; parameters = structuredClone(response.profile ?? scan.parameters); message = response.profile ? 'このソースと一致する保存プロファイルを適用しました。' : 'コメントから検出しました。探索範囲を確認してください。' }) }
  async function setup() { await action(async () => { environment = await post<TuneEnvironment>('environment/setup'); message = '専用環境の準備ができました。' }) }
  async function saveProfile() { await action(async () => { parameters = await requestJSON<TuneParameter[]>('/api/tuning/profiles', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ solver, hash: scan?.hash, parameters }) }); message = 'このソース用の探索設定を保存しました。' }) }
  async function start() { await action(async () => { const s = await post<TuneStudy>('studies', { solver, input_dir: inputDir, source_hash: scan?.hash, parameters, trials, threads, timeout_ms: timeoutMS, seconds, seed }); await loadStudies(); await selectStudy(s.id) }) }
  async function refresh(id: string) {
    if (refreshing || destroyed) return; refreshing = true
    try {
      const incoming = await requestJSON<TuneDetail>(`/api/tuning/studies/${id}`)
      if (id !== selectedStudyID || destroyed) return
      detail = incoming
      studies=studies.map(study=>study.id===id ? incoming.study : study)
      if(environment?.available && !environment.ready) environment=await requestJSON<TuneEnvironment>("/api/tuning/environment")
      rows = await requestJSON<TuneTrial[]>(`/api/tuning/studies/${id}/trials?offset=${page * 50}&limit=50`)
      const count = detail.study.completed + detail.study.failed
      const revision = `${id}:${count}`
      if (revision !== lastRevision) { const all: TuneTrial[] = []; for (let offset = 0; offset < count; offset += 100) all.push(...await requestJSON<TuneTrial[]>(`/api/tuning/studies/${id}/trials?offset=${offset}&limit=100`)); history = all; lastRevision = revision }
    } finally { refreshing = false }
  }
  async function selectStudy(id: string) {
    events?.close(); selectedStudyID = id; page = 0; selectedTrial = null; lastRevision = ''; while (refreshing) await new Promise(resolve => setTimeout(resolve, 30)); await refresh(id)
    if (destroyed) return; events = new EventSource(`/api/tuning/studies/${id}/events`)
    events.onopen = () => { void refresh(id).catch(e => message = errorMessage(e)) }
    events.addEventListener('status', () => { void refresh(id).catch(e => message = errorMessage(e)) })
  }
  async function stop(immediate: boolean) { if (!detail) return; await action(async () => { await post(`studies/${detail!.study.id}/${immediate ? 'cancel' : 'pause'}`); message = immediate ? '評価を中断しています。' : '現在の候補の評価後に停止します。' }) }
  async function resume() { if (!detail) return; await action(async () => { await post(`studies/${detail!.study.id}/resume`, { additional_trials: additional }); additional = 0; await refresh(detail!.study.id) }) }
  async function exportSource() { if (!detail) return; await action(async () => { const result = await post<{ path: string; source: string }>(`studies/${detail!.study.id}/export`, { number: selectedTrial }); const url = URL.createObjectURL(new Blob([result.source], { type: 'text/plain;charset=utf-8' })); const a = document.createElement('a'); a.href = url; a.download = result.path.split('/').pop() ?? 'main.best.cpp'; a.click(); URL.revokeObjectURL(url); message = `保存しました: ${result.path}` }) }
  async function validate() { if (!detail) return; await action(async () => { await post(`studies/${detail!.study.id}/validate`, { input_dir: validationInput, number: selectedTrial, threads: validationThreads }); message = '別入力で現在値と候補を評価しています。'; await refresh(detail!.study.id) }) }
  onMount(() => { void Promise.all([loadStudies(), requestJSON<TuneEnvironment>('/api/tuning/environment').then(v => environment = v)]).catch(e => message = errorMessage(e)); return () => { destroyed = true; events?.close() } })
</script>

<div class="tuning">
  {#if message}<p class="notice" role="status">{message}</p>{/if}
  <details class="environment">
    <summary>専用Python・Optuna：{environment?.available ? environment.ready ? '準備済み' : '同梱済み（初回利用時に自動展開）' : '未同梱の開発ビルド'}</summary>
    {#if environment?.available}
      <p>Python {environment.python} / Optuna {environment.optuna} を同梱しています。追加インストール・ダウンロードは不要です。システムのPython設定は変更しません。</p>
      <p>同梱データ {mb(environment.compressed_bytes)} ＋ 展開後 {mb(environment.unpacked_bytes)}。実行結果の容量は別途増えます。</p>
      <p class="path">保存先：{environment.path}</p>
      <p>専用環境の削除はGUIを終了して <code>ahc-plaza tune clean-runtime</code>。探索履歴は残ります。</p>
      <button disabled={busy || active} onclick={setup}>{busy ? '準備中…' : '専用環境を確認・展開'}</button>
    {:else}<p>配布版を利用するか、開発環境で <code>make build</code> を実行してください。通常のRunはそのまま利用できます。</p>{/if}
  </details>

  <section>
    <h2>新しいチューニング</h2>
    <p>定数に <code>// @tune 10 200</code> を付け、探索範囲を確認して開始します。元のソースは変更しません。</p>
    <div class="fields">
      <label>ソース<select aria-label="ソース" bind:value={solver} onchange={() => { scan = null; parameters = [] }}><option value="">選択してください</option>{#each solvers as path}<option value={path}>{path}</option>{/each}</select></label>
      <label>入力セット<select aria-label="入力セット" bind:value={inputDir}><option value="">選択してください</option>{#each inputDirectories as path}<option value={path}>{path}</option>{/each}</select></label>
      <button disabled={busy || !solver} onclick={detect}>パラメータを検出</button>
    </div>
    {#if scan}
      {#if previousParameters.length}<details open><summary>前回の保存時からソースが変更されています。以前の範囲は自動適用していません。</summary><div class="table-scroll"><table><thead><tr><th>名前</th><th>以前の宣言</th><th>現在の宣言</th><th>以前の範囲</th></tr></thead><tbody>{#each previousParameters as old}<tr><td>{old.name}</td><td>{old.type} = {old.token}</td><td>{scan.parameters.find(p=>p.name===old.name)?.token ?? '見つかりません'}</td><td>{old.low} ～ {old.high}</td></tr>{/each}</tbody></table></div></details>{/if}
      <div class="table-scroll"><table><thead><tr><th>探索</th><th>名前・型</th><th>現在値</th><th>下限</th><th>上限</th><th>刻み</th><th>対数</th></tr></thead><tbody>
        {#each parameters as p}<tr><td><input type="checkbox" aria-label={`${p.name}を探索`} bind:checked={p.enabled} /></td><td><button class="link" onclick={() => { const line = document.getElementById(`tune-source-${p.line}`); const block = line?.closest("details"); if (block) block.open = true; line?.scrollIntoView({ block: "center" }) }}>{p.name}</button><small>{p.type} · {p.line}行</small></td><td>{number(p.current)}</td>
          <td><input type="number" step="any" aria-label={`${p.name}の下限`} value={p.low ?? ''} oninput={e => { p.low = e.currentTarget.value === '' ? null : +e.currentTarget.value; parameters = parameters }} /></td>
          <td><input type="number" step="any" aria-label={`${p.name}の上限`} value={p.high ?? ''} oninput={e => { p.high = e.currentTarget.value === '' ? null : +e.currentTarget.value; parameters = parameters }} /></td>
          <td><input type="number" step="any" aria-label={`${p.name}の刻み`} disabled={p.log} value={p.step ?? ''} oninput={e => { p.step = e.currentTarget.value === '' ? null : +e.currentTarget.value; parameters = parameters }} /></td>
          <td><input type="checkbox" aria-label={`${p.name}を対数探索`} bind:checked={p.log} onchange={() => { if (p.log) p.step = null; parameters = parameters }} /></td></tr>{/each}
      </tbody></table></div>
      <div class="actions"><button disabled={busy || incomplete} onclick={saveProfile}>探索設定を保存</button><button onclick={() => parameters = structuredClone(scan!.parameters)}>コメントの設定に戻す</button></div>
      <details><summary>検出したソース</summary><pre>{#each scan.source.split('\n') as line, i}<span id={`tune-source-${i + 1}`} class:annotated={parameters.some(p => p.line === i + 1)}>{i + 1}  {line}{'\n'}</span>{/each}</pre></details>
    {/if}
    <div class="fields"><label>追加候補の試行回数<input type="number" min="1" max="10000" bind:value={trials} /></label><p>目的値：生スコア平均（{objective === 'min' ? '最小化' : '最大化'}）</p></div>
    <details><summary>詳細設定</summary><div class="fields"><label>ケース並列数（0＝自動）<input type="number" min="0" max="256" bind:value={threads} /></label><label>タイムアウト ms（0＝設定値）<input type="number" min="0" bind:value={timeoutMS} /></label><label>時間予算 秒（0＝制限なし）<input type="number" min="0" bind:value={seconds} /></label><label>Optuna seed<input type="number" min="0" bind:value={seed} /></label></div><p>時間予算は次の候補の開始を制限します。実行中の候補は終了まで待ちます。</p></details>
    <p>{trials || 0}候補 × {caseCount ?? "—"}ケース、加えて現在値{caseCount ?? "—"}ケースを評価します。失敗候補も回数に含みます。候補ごとに再コンパイルします。</p>
    <button class="primary" disabled={busy || active || incomplete || !environment?.available || !inputDir || !trials || caseCount === 0} onclick={start}>{busy ? '準備中…' : 'チューニングを開始'}</button>
  </section>

  <section><div class="heading"><h2>保存したチューニング</h2><button onclick={() => action(loadStudies)}>一覧を更新</button></div>
    <label>Study<select aria-label="Study" value={detail?.study.id ?? ''} onchange={e => action(() => selectStudy(e.currentTarget.value))}><option value="" disabled>選択してください</option>{#each studies as s}<option value={s.id}>{s.id} · {states[s.status] ?? s.status}</option>{/each}</select></label>
  </section>
  {#if detail}
    <section>
      <div class="heading"><h2>{states[detail.study.status] ?? detail.study.status}</h2><span>{detail.study.completed + detail.study.failed} / {detail.study.trials}候補</span></div>
      {#if detail.study.error}<p class="notice">{detail.study.error}</p>{#if detail.worker_log}<details><summary>Optunaのログ</summary><pre>{detail.worker_log}</pre></details>{/if}{/if}
      <p>成功 {detail.study.completed} · 失敗 {detail.study.failed} · 経過時間 {Math.round(detail.study.elapsed)}秒 · 使用容量 {mb(detail.usage_bytes)}</p>
      <p>推定残り時間：{detail.study.completed + detail.study.failed > 0 ? `${Math.ceil(detail.study.elapsed / (detail.study.completed + detail.study.failed + 1) * Math.max(0, detail.study.trials - detail.study.completed - detail.study.failed))}秒（ビルドを含む概算）` : "現在値の評価後に算出"}</p>
      <p>固定条件：{detail.manifest.request.solver} / {detail.manifest.request.input_dir} / {detail.manifest.prepared.inputs.length}ケース / 並列 {detail.manifest.request.threads} / {detail.manifest.request.timeout_ms} ms</p>
      <div class="metrics"><div>現在値<strong>{number(detail.study.baseline_value)}</strong></div><div>{detail.study.best_run === detail.study.baseline_run ? '現在値が最良' : '最良値'}<strong>{number(detail.study.best_value)}</strong></div><div>現在値との差<strong>{baseline !== null && detail.study.best_value !== null ? number(detail.study.best_value - baseline) : '—'}</strong></div></div>
      <div class="actions">{#if active}<button disabled={busy} onclick={() => stop(false)}>候補の終了後に一時停止</button><button disabled={busy} onclick={() => stop(true)}>今すぐ停止</button>{:else}<label>追加試行数<input type="number" min="0" max="10000" bind:value={additional} /></label><button disabled={busy} onclick={resume}>保存したソースで再開</button>{/if}</div>
      {#if chartPoints.length}<figure><svg viewBox="0 0 780 190" role="img" aria-label="候補の目的値と最良値の推移"><line x1="35" y1="155" x2="745" y2="155" stroke="currentColor" opacity=".25" /><text x="35" y="16">{number(hi)}</text><text x="35" y="180">{number(lo)}</text><polyline points={bestLine} fill="none" stroke="var(--selection, #586f55)" stroke-width="2" />{#each chartPoints as p}<circle cx={p.x} cy={p.y} r="3" fill="currentColor"><title>Trial {p.trial.number}: {p.trial.value}</title></circle>{/each}</svg><figcaption>点：各候補の生スコア平均 ／ 線：それまでの最良値。失敗は数値に含めません。</figcaption></figure>{/if}
      <div class="table-scroll"><table><thead><tr><th>Trial</th><th>状態</th><th>目的値</th><th>パラメータ</th><th>Run・失敗理由</th></tr></thead><tbody>{#each rows as t}<tr><td>{t.number}</td><td>{states[t.status] ?? t.status}</td><td>{number(t.value)}</td><td><code>{JSON.stringify(t.params)}</code></td><td>{#if t.run_id}<button class="link" onclick={() => onOpenRun(t.run_id)}>Runの詳細</button>{/if}{#if t.reason}<p>{t.reason}</p>{/if}{#if t.status === 'COMPLETE'}<button onclick={() => selectedTrial = t.number}>この候補を選択</button>{/if}</td></tr>{/each}</tbody></table></div>
      <div class="actions"><button disabled={page === 0 || refreshing} onclick={() => { page--; void refresh(detail!.study.id).catch(e => message = errorMessage(e)) }}>前へ</button><span>{page + 1}ページ</span><button disabled={rows.length < 50 || refreshing} onclick={() => { page++; void refresh(detail!.study.id).catch(e => message = errorMessage(e)) }}>次へ</button></div>
      <p>選択中：{selectedTrial === null ? '現在値を含めた最良候補' : `Trial ${selectedTrial}`}</p>
      <div class="actions"><button onclick={() => selectedTrial = null}>最良候補に戻す</button><button disabled={busy || detail.study.best_value === null} onclick={exportSource}>値を入れたC++を保存</button><button disabled={!detail.study.best_run} onclick={() => onCompare(detail!.study.baseline_run, detail!.study.best_run)}>現在値と最良候補を比較</button></div>
      <h3>別入力セットで検証</h3><p>探索に使っていないケースで、現在値と選択した候補を比較します。検証結果はOptunaへ戻しません。</p>
      <div class="fields"><label>検証用入力<select aria-label="検証用入力" bind:value={validationInput}><option value="">選択してください</option>{#each inputDirectories as path}<option value={path}>{path}</option>{/each}</select></label><label>ケース並列数<input type="number" min="1" max="256" bind:value={validationThreads} /></label><button disabled={busy || active || !validationInput || detail.study.best_value === null} onclick={validate}>現在値と候補を検証</button></div>
      {#each detail.study.validations ?? [] as v}<p>{v.input_dir} {#if v.error}<span>{v.error}</span>{:else}<button class="link" onclick={() => onCompare(v.baseline_run, v.candidate_run)}>検証結果を比較</button>{/if}</p>{/each}
    </section>
  {/if}
</div>

<style>
  .tuning { max-width: 1180px; margin: 0 auto; display: grid; gap: 24px; }
  section, .environment { border: 1px solid var(--rule); padding: 22px; background: var(--paper); }
  h2 { margin: 0 0 12px; font-size: 18px; } h3 { margin-top: 24px; font-size: 15px; }
  p { color: var(--pencil); font-size: 13px; line-height: 1.7; overflow-wrap: anywhere; }
  label { display: grid; gap: 7px; font-size: 12px; min-width: 0; }
  input:not([type=checkbox]), select { padding: 8px; border: 1px solid var(--rule); background: var(--paper); color: var(--graphite); min-width: 0; width: 100%; box-sizing: border-box; }
  .fields, .actions, .heading { display: flex; flex-wrap: wrap; gap: 14px; align-items: end; margin: 16px 0; }
  .fields label { flex: 1 1 200px; } .heading { justify-content: space-between; align-items: center; }
  button { padding: 8px 12px; border: 1px solid var(--rule); background: var(--paper-shade); color: var(--graphite); font-size: 12px; }
  input[type=checkbox] { width: 16px; height: 16px; min-height: 16px; padding: 0; margin: 0; }
  button:disabled { opacity: .5; } .primary { background: var(--graphite); color: var(--paper); padding: 11px 20px; }
  .link { border: 0; background: none; padding: 2px 0; text-decoration: underline; }
  details { margin: 14px 0; } summary { cursor: pointer; font-size: 13px; } .table-scroll { overflow-x: auto; }
  table { width: 100%; border-collapse: collapse; font-size: 12px; } td, th { text-align: left; padding: 9px; border-bottom: 1px solid var(--rule); } td input[type=number] { min-width: 95px; }
  small { display: block; color: var(--pencil); margin-top: 4px; } pre { overflow: auto; max-height: 350px; font: 12px/1.6 var(--mono); padding: 12px; background: var(--paper-shade); } .annotated { background: #ddddaa44; }
  .metrics { display: flex; flex-wrap: wrap; gap: 30px; margin: 22px 0; font-size: 12px; } strong { display: block; font: 24px var(--mono); margin-top: 8px; }
  figure { margin: 20px 0; } svg { width: 100%; max-height: 240px; } svg text { font-size: 11px; fill: currentColor; } figcaption { font-size: 11px; color: var(--pencil); }
  @media(max-width: 650px) { section, .environment { padding: 14px; } .fields { display: grid; grid-template-columns: 1fr; } }
</style>

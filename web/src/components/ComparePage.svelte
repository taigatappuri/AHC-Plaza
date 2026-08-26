<script lang="ts">
  import { onMount } from 'svelte'
  import { errorMessage, requestJSON } from '../lib/api'
  import { comparisonCasesForResult } from '../lib/comparison-cases'
  import { formatScore, runLabel } from '../lib/formatters'
  import type { CaseResult, Comparison, InputFeatureDefinition, Run, FeatureCondition, FeatureConditionOperator } from '../lib/types'
  import ComparisonHistogram from './ComparisonHistogram.svelte'
  import VisualizerPanel from './VisualizerPanel.svelte'

  type DraftCondition = Omit<FeatureCondition, 'value'> & { id: number; value?: number }
  type RunCasesResponse = { cases: CaseResult[] }
  type ComparisonCase = { id: string; a: CaseResult; b: CaseResult }

  export let runs: Run[] = []
  export let compareA = ''
  export let compareB = ''
  export let comparison: Comparison | null = null
  export let features: InputFeatureDefinition[] = []
  export let objective: 'max' | 'min' = 'max'
  export let loading = false
  export let onCompare: (runA: string, runB: string, conditions: FeatureCondition[]) => void | Promise<void> = () => {}
  export let onInvalidate: () => void = () => {}
  export let onConfigureInputFormat: () => void = () => {}

  const operators: { value: FeatureConditionOperator; label: string }[] = [
    { value: '<', label: '<' }, { value: '<=', label: '≤' }, { value: '=', label: '=' },
    { value: '!=', label: '≠' }, { value: '>=', label: '≥' }, { value: '>', label: '>' }
  ]
  let conditions: DraftCondition[] = []
  let nextConditionID = 1
  let comparisonCases: ComparisonCase[] = []
  let selectedCaseID = ''
  let caseLoading = false
  let caseError = ''
  let requestedPair = ''
  let caseRequestSerial = 0
  let visualizerReady = false
  let visualizerLoading = true
  let visualizerError = ''

  const isReady = (condition: DraftCondition): condition is DraftCondition & { value: number } =>
    Boolean(condition.feature) && typeof condition.value === 'number' && Number.isFinite(condition.value)
  const runOption = (run: Run) => `${runLabel(run.run_number, run.id)} · ${run.solver_path} · 平均 ${formatScore(run.average_score)}`
  const featureOption = (feature: InputFeatureDefinition) => feature.kind === 'cpp'
    ? `${feature.name} · C++`
    : `${feature.name} · ${feature.line}:${feature.column}`
  const conditionText = (result: Comparison) => result.filter.conditions
    .map((condition) => `${condition.feature} ${condition.operator} ${condition.value}`)
    .join(' AND ')
  const verdict = (result: Comparison) => !result.significant
    ? '有意差なし'
    : result.confidence_high < 0 ? 'Run B優位' : result.confidence_low > 0 ? 'Run A優位' : '有意差あり'
  const signedScore = (value: number) => `${value > 0 ? '+' : ''}${formatScore(value)}`
  const signedPercent = (value: number) => `${value > 0 ? '+' : ''}${(value * 100).toFixed(2)}%`
  const differenceLabel = () => objective === 'max' ? 'A − B' : 'B − A'
  const pValue = (value: number) => value < 0.001 ? 'p < 0.001' : `p = ${value.toFixed(3)}`

  $: runA = runs.find((run) => run.id === compareA) ?? null
  $: runB = runs.find((run) => run.id === compareB) ?? null
  $: selectableComparisonCases = comparisonCasesForResult(comparisonCases, comparison)
  $: selectedComparisonCase = selectableComparisonCases.find((item) => item.id === selectedCaseID) ?? selectableComparisonCases[0] ?? null
  $: displayedSelectedCaseID = selectedComparisonCase?.id ?? ''
  $: runPair = compareA && compareB ? `${compareA}:${compareB}` : ''
  $: if (runPair && runPair !== requestedPair) {
    requestedPair = runPair
    void loadComparisonCases(compareA, compareB, runPair)
  }
  $: if (!runPair && requestedPair) {
    requestedPair = ''
    caseRequestSerial += 1
    comparisonCases = []
    selectedCaseID = ''
    caseError = ''
    caseLoading = false
  }

  onMount(() => {
    void loadVisualizerStatus()
  })

  async function loadVisualizerStatus() {
    visualizerLoading = true
    visualizerError = ''
    try {
      const status = await requestJSON<{ ready: boolean }>('/api/visualizer')
      visualizerReady = status.ready
    } catch (reason) {
      visualizerReady = false
      visualizerError = errorMessage(reason)
    } finally {
      visualizerLoading = false
    }
  }

  async function loadComparisonCases(runAID: string, runBID: string, pair: string) {
    const serial = ++caseRequestSerial
    caseLoading = true
    caseError = ''
    comparisonCases = []
    selectedCaseID = ''
    try {
      const [detailA, detailB] = await Promise.all([
        requestJSON<RunCasesResponse>(`/api/runs/${encodeURIComponent(runAID)}`),
        requestJSON<RunCasesResponse>(`/api/runs/${encodeURIComponent(runBID)}`)
      ])
      if (serial !== caseRequestSerial || pair !== runPair) return
      const casesB = new Map(detailB.cases.map((item) => [item.input_case_id, item]))
      comparisonCases = detailA.cases.flatMap((a) => {
        const b = casesB.get(a.input_case_id)
        return b ? [{ id: a.input_case_id, a, b }] : []
      })
      selectedCaseID = comparisonCases[0]?.id ?? ''
    } catch (reason) {
      if (serial === caseRequestSerial && pair === runPair) caseError = errorMessage(reason)
    } finally {
      if (serial === caseRequestSerial && pair === runPair) caseLoading = false
    }
  }

  function addCondition() {
    if (!features[0] || conditions.length >= 32) return
    conditions = [...conditions, { id: nextConditionID++, feature: features[0].name, operator: '<=' }]
    onInvalidate()
  }

  function removeCondition(id: number) {
    conditions = conditions.filter((condition) => condition.id !== id)
    onInvalidate()
  }

  function submit() {
    if (!compareA || !compareB || !conditions.every(isReady)) return
    void onCompare(compareA, compareB, conditions.filter(isReady).map(({ feature, operator, value }) => ({ feature, operator, value })))
  }
</script>

<section class="compare-panel">
  <form class="compare-form" onsubmit={(event) => { event.preventDefault(); submit() }}>
    <label class="run-picker run-a-picker"><span class="field-label run-label"><i aria-hidden="true">A</i>Run A</span><select bind:value={compareA} onchange={onInvalidate}><option value="">Runを選択</option>{#each runs as run}<option value={run.id}>{runOption(run)}</option>{/each}</select></label>
    <span class="versus">vs</span>
    <label class="run-picker run-b-picker"><span class="field-label run-label"><i aria-hidden="true">B</i>Run B</span><select bind:value={compareB} onchange={onInvalidate}><option value="">Runを選択</option>{#each runs as run}<option value={run.id}>{runOption(run)}</option>{/each}</select></label>
    <button class="primary-action" type="submit" disabled={!compareA || !compareB || !conditions.every(isReady) || loading}>{loading ? '比較中…' : '比較'}</button>
  </form>

  <div class="condition-editor">
    <header><strong>入力条件</strong><span>{conditions.length ? `${conditions.length}条件（AND）` : '全ケース'}</span></header>
    {#if features.length === 0}
      <p>入力特徴量未設定 <button type="button" class="text-action" onclick={onConfigureInputFormat}>設定</button></p>
    {:else}
      {#each conditions as condition, index (condition.id)}
        <div class="condition-row">
          <span>{index + 1}</span>
          <select bind:value={condition.feature} onchange={onInvalidate} aria-label={`条件${index + 1}の特徴量`}>{#each features as feature}<option value={feature.name}>{featureOption(feature)}</option>{/each}</select>
          <select bind:value={condition.operator} onchange={onInvalidate} aria-label={`条件${index + 1}の演算子`}>{#each operators as operator}<option value={operator.value}>{operator.label}</option>{/each}</select>
          <input type="number" step="any" bind:value={condition.value} oninput={onInvalidate} aria-label={`条件${index + 1}の値`} />
          <button class="remove-action" type="button" onclick={() => removeCondition(condition.id)} aria-label={`条件${index + 1}を削除`}>削除</button>
        </div>
      {/each}
      <button type="button" class="text-action" onclick={addCondition} disabled={conditions.length >= 32}>条件を追加</button>
    {/if}
  </div>

  {#if comparison}
    <div class="comparison-result">
      <header class="result-lead">
        <div><span>{differenceLabel()}</span><strong>{signedScore(comparison.mean_improvement)}</strong><em>{signedPercent(comparison.improvement_rate)}</em></div>
        <span class:run-a-verdict={comparison.significant && comparison.confidence_low > 0} class:run-b-verdict={comparison.significant && comparison.confidence_high < 0} class="verdict">{verdict(comparison)}</span>
      </header>
      <div class="result-evidence">
        <span>{Math.round(comparison.confidence_level * 100)}%信頼区間 {signedScore(comparison.confidence_low)} ～ {signedScore(comparison.confidence_high)}</span>
        <span>{pValue(comparison.p_value)}</span>
        <span>{comparison.filter.matched_case_count} / {comparison.filter.original_case_count}ケース</span>
      </div>
      {#if comparison.filter.active}<code>{conditionText(comparison)}</code>{/if}
      <dl class="run-means">
        <div class="run-a-mean"><dt><i aria-hidden="true">A</i>Run A 平均</dt><dd>{formatScore(comparison.mean_a)}</dd></div>
        <div class="run-b-mean"><dt><i aria-hidden="true">B</i>Run B 平均</dt><dd>{formatScore(comparison.mean_b)}</dd></div>
      </dl>
      <ComparisonHistogram {comparison} />
    </div>
  {:else}
    <div class="empty-state compact"><strong>比較を実行すると結果を表示します</strong></div>
  {/if}

  <section class="comparison-visualizer-section">
    <header>
      <div><h2>ケースをビジュアライザで比較</h2><p>同じケース ID の各 Run の入力と出力を表示します。</p></div>
      {#if selectedComparisonCase}<span class="count-badge">{comparison?.filter.active ? '対象' : '共通'} {selectableComparisonCases.length}件</span>{/if}
    </header>

    {#if !compareA || !compareB}
      <div class="empty-state compact"><strong>Run A と Run B を選択してください</strong></div>
    {:else if caseLoading}
      <div class="empty-state compact"><strong>共通ケースを読み込み中…</strong></div>
    {:else if caseError}
      <div class="visualizer-error">共通ケースを取得できません: {caseError}</div>
    {:else if selectableComparisonCases.length === 0}
      <div class="empty-state compact"><strong>{comparison?.filter.active ? '入力条件に一致する共通ケースがありません' : '2つの Run に共通するケースがありません'}</strong></div>
    {:else}
      <div class="comparison-case-picker">
        <label>
          <span class="field-label">ケース</span>
          <select value={displayedSelectedCaseID} onchange={(event) => selectedCaseID = event.currentTarget.value} aria-label="比較するケース">
            {#each selectableComparisonCases as item}
              <option value={item.id}>{item.id} · A {formatScore(item.a.score)} / B {formatScore(item.b.score)} · seed {item.a.seed}</option>
            {/each}
          </select>
        </label>
        {#if selectedComparisonCase}
          <div class="comparison-case-meta"><span>Run A</span><strong>{formatScore(selectedComparisonCase.a.score)}</strong><span>Run B</span><strong>{formatScore(selectedComparisonCase.b.score)}</strong></div>
        {/if}
      </div>

      {#if visualizerLoading}
        <div class="empty-state compact"><strong>ビジュアライザを確認中…</strong></div>
      {:else if visualizerError}
        <div class="visualizer-error">ビジュアライザの状態を取得できません: {visualizerError}</div>
      {:else if !visualizerReady}
        <div class="empty-state compact"><strong>公式ビジュアライザが未設定です</strong><span>実行詳細画面から設定してください。</span></div>
      {:else if runA && runB && selectedComparisonCase}
        <div class="comparison-visualizer-grid">
          <VisualizerPanel selectedRun={runA} caseResults={[selectedComparisonCase.a]} selectedCaseID={displayedSelectedCaseID} title={`Run A · ${runLabel(runA.run_number, runA.id)}`} comparisonMode />
          <VisualizerPanel selectedRun={runB} caseResults={[selectedComparisonCase.b]} selectedCaseID={displayedSelectedCaseID} title={`Run B · ${runLabel(runB.run_number, runB.id)}`} comparisonMode />
        </div>
      {/if}
    {/if}
  </section>
</section>

<style>
  .compare-form { display: grid; grid-template-columns: 1fr 36px 1fr 100px; align-items: end; gap: 12px; padding-bottom: 20px; }
  .compare-form label { display: grid; gap: 7px; }
  .run-label { display: flex; align-items: center; gap: 7px; }
  .run-label i, .run-means i { display: inline-grid; place-items: center; width: 20px; height: 20px; color: #fff; border-radius: 3px; font: normal 600 11px var(--mono); }
  .run-a-picker .run-label { color: var(--run-a); }
  .run-b-picker .run-label { color: var(--run-b); }
  .run-a-picker .run-label i, .run-a-mean i { background: var(--run-a); }
  .run-b-picker .run-label i, .run-b-mean i { background: var(--run-b); }
  .run-a-picker select { border-color: color-mix(in srgb, var(--run-a) 52%, var(--rule-strong)); box-shadow: inset 3px 0 var(--run-a); }
  .run-b-picker select { border-color: color-mix(in srgb, var(--run-b) 52%, var(--rule-strong)); box-shadow: inset 3px 0 var(--run-b); }
  .versus { padding-bottom: 11px; color: var(--pencil); font: 12px var(--mono); text-align: center; }
  .condition-editor { display: grid; gap: 10px; padding: 16px 20px; background: var(--paper); }
  .condition-editor header { display: flex; justify-content: space-between; color: var(--graphite-soft); font-size: 12px; }
  .condition-editor header span, .condition-editor p { margin: 0; color: var(--pencil); font-size: 12px; }
  .condition-row { display: grid; grid-template-columns: 24px 1fr 90px 140px 48px; align-items: center; gap: 8px; }
  .condition-row > span { color: var(--pencil); font: 11px var(--mono); text-align: center; }
  .comparison-result { margin-top: 16px; background: var(--paper); }
  .compare-panel > .empty-state { margin-top: 16px; background: var(--paper); }
  .result-lead { display: flex; align-items: center; justify-content: space-between; padding: 18px 20px 10px; }
  .result-lead > div { display: flex; align-items: baseline; gap: 10px; }
  .result-lead div > span { color: var(--pencil); font: 11px var(--mono); }
  .result-lead strong { color: var(--graphite); font: 24px var(--mono); }
  .result-lead em { color: var(--graphite-soft); font: normal 13px var(--mono); }
  .verdict { padding: 8px 12px; color: var(--graphite-soft); background: var(--paper-shade); border-radius: 4px; font-size: 18px; font-weight: 600; line-height: 1.25; }
  .verdict.run-a-verdict { color: var(--run-a); background: var(--run-a-soft); }
  .verdict.run-b-verdict { color: var(--run-b); background: var(--run-b-soft); }
  .result-evidence { display: flex; gap: 18px; padding: 0 20px 16px; color: var(--pencil); font: 11px var(--mono); }
  .comparison-result > code { display: block; padding: 10px 20px; color: var(--graphite-soft); background: var(--paper-shade); font: 11px var(--mono); }
  .run-means { display: grid; grid-template-columns: repeat(2, 1fr); margin: 0; border-top: 1px solid var(--rule); }
  .run-means div { padding: 14px 20px; border-right: 1px solid var(--rule); }
  .run-means div:last-child { border-right: 0; }
  .run-means dt { display: flex; align-items: center; gap: 7px; font-size: 12px; font-weight: 600; }
  .run-means .run-a-mean dt, .run-means .run-a-mean dd { color: var(--run-a); }
  .run-means .run-b-mean dt, .run-means .run-b-mean dd { color: var(--run-b); }
  .run-means dd { margin: 6px 0 0; color: var(--graphite); font: 15px var(--mono); }
  .comparison-visualizer-section { margin-top: 16px; background: var(--paper); }
  .comparison-visualizer-section > header { display: flex; align-items: center; justify-content: space-between; gap: 16px; min-height: 60px; padding: 12px 20px; border-bottom: 1px solid var(--rule); }
  .comparison-visualizer-section h2 { margin: 0; font-size: 15px; }
  .comparison-visualizer-section header p { margin: 4px 0 0; color: var(--pencil); font-size: 11px; }
  .comparison-case-picker { display: flex; align-items: end; justify-content: space-between; gap: 16px; padding: 12px 20px; border-bottom: 1px solid var(--rule); }
  .comparison-case-picker label { display: grid; min-width: min(100%, 480px); gap: 6px; }
  .comparison-case-meta { display: grid; grid-template-columns: auto auto auto auto; gap: 6px 10px; color: var(--pencil); font: 11px var(--mono); }
  .comparison-case-meta strong { color: var(--graphite-soft); font-weight: 600; }
  .comparison-visualizer-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 1px; background: var(--rule); }
  .comparison-visualizer-grid :global(.visualizer-panel) { min-width: 0; }
  .comparison-visualizer-grid :global(.visualizer-toolbar) { grid-template-columns: 72px minmax(0, 1fr); }
  .comparison-visualizer-grid :global(.visualizer-scale) { grid-column: auto; }

  @media (max-width: 1100px) {
    .compare-form { grid-template-columns: 1fr 36px 1fr; }
    .compare-form .primary-action { grid-column: 1 / -1; }
    .comparison-visualizer-grid { grid-template-columns: 1fr; }
  }
</style>

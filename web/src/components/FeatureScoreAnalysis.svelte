<script lang="ts">
  import { filterCaseResultsByFeatures } from '../lib/feature-filter'
  import { formatDuration, formatScore, statusLabel } from '../lib/formatters'
  import type { CaseResult, InputFeatureDefinition, FeatureConditionOperator, FeatureData } from '../lib/types'

  type Condition = { id: number; feature: string; operator: FeatureConditionOperator; value?: number }

  export let caseResults: CaseResult[] = []
  export let featureData: FeatureData | null = null
  export let onSelectCase: (caseID: string) => void | Promise<void> = () => {}
  export let onConfigureInputFormat: () => void = () => {}
  export let onFilteredCaseResultsChange: (caseResults: CaseResult[] | null) => void = () => {}

  const operators: FeatureConditionOperator[] = ['<', '<=', '=', '!=', '>=', '>']
  let conditions: Condition[] = []
  let nextID = 1
  let featuresKey = ''

  const label = (feature: InputFeatureDefinition) => feature.kind === 'cpp' ? `${feature.name} · C++` : `${feature.name} · ${feature.line}:${feature.column}`
  const ready = (condition: Condition): condition is Condition & { value: number } =>
    typeof condition.value === 'number' && Number.isFinite(condition.value)
  const formatValue = (value?: number) => value == null ? '—' : value.toLocaleString('ja-JP', { maximumFractionDigits: 6 })

  function addCondition() {
    if (!features[0] || conditions.length >= 32) return
    conditions = [...conditions, { id: nextID++, feature: features[0].name, operator: '<=' }]
  }

  function removeCondition(id: number) {
    conditions = conditions.filter((condition) => condition.id !== id)
  }

  function handleKey(event: KeyboardEvent, caseID: string) {
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    void onSelectCase(caseID)
  }

  $: features = featureData?.features ?? []
  $: nextFeaturesKey = features.map((feature) => feature.name).join('\n')
  $: if (nextFeaturesKey !== featuresKey) {
    featuresKey = nextFeaturesKey
    conditions = features[0] ? [{ id: nextID++, feature: features[0].name, operator: '<=' }] : []
  }
  $: readyConditions = conditions.filter(ready)
  $: analysisReady = conditions.length > 0 && readyConditions.length === conditions.length
  $: valuesByCase = new Map((featureData?.cases ?? []).map((item) => [item.input_case_id, item.values]))
  $: matched = analysisReady
    ? filterCaseResultsByFeatures(caseResults, featureData?.cases ?? [], readyConditions)
    : []
  $: onFilteredCaseResultsChange(analysisReady ? matched : null)
  $: scores = matched.map((result) => result.score).filter(Number.isFinite)
  $: mean = scores.length ? scores.reduce((sum, score) => sum + score, 0) / scores.length : undefined
  $: selectedFeatures = [...new Set(readyConditions.map((condition) => condition.feature))]
</script>

<section class="analysis-panel">
  <div class="section-heading"><h2>入力特徴量</h2></div>
  {#if featureData === null}
    <div class="empty-state compact"><strong>入力特徴量を解析中</strong></div>
  {:else if features.length === 0}
    <div class="empty-state compact"><strong>入力特徴量未設定</strong><button class="text-action" onclick={onConfigureInputFormat}>設定</button></div>
  {:else}
    <div class="conditions">
      {#each conditions as condition, index (condition.id)}
        <div class="condition">
          <span>{index + 1}</span>
          <select bind:value={condition.feature} aria-label={`条件${index + 1}の特徴量`}>{#each features as feature}<option value={feature.name}>{label(feature)}</option>{/each}</select>
          <select bind:value={condition.operator} aria-label={`条件${index + 1}の演算子`}>{#each operators as operator}<option value={operator}>{operator}</option>{/each}</select>
          <input type="number" step="any" bind:value={condition.value} aria-label={`条件${index + 1}の値`} />
          <button class="remove-action" onclick={() => removeCondition(condition.id)} aria-label={`条件${index + 1}を削除`}>削除</button>
        </div>
      {/each}
      <button class="text-action" onclick={addCondition}>AND条件を追加</button>
    </div>

    {#if analysisReady}
      <dl class="analysis-summary">
        <div><dt>対象</dt><dd>{matched.length} / {caseResults.length}</dd></div>
        <div><dt>平均</dt><dd>{formatScore(mean)}</dd></div>
        <div><dt>最小</dt><dd>{formatScore(scores.length ? Math.min(...scores) : undefined)}</dd></div>
        <div><dt>最大</dt><dd>{formatScore(scores.length ? Math.max(...scores) : undefined)}</dd></div>
      </dl>
      <div class="analysis-table">
        <table><thead><tr><th>ケース</th>{#each selectedFeatures as feature}<th>{feature}</th>{/each}<th>スコア</th><th>時間</th><th>状態</th></tr></thead>
          <tbody>{#each matched as result}<tr role="button" tabindex="0" onclick={() => onSelectCase(result.input_case_id)} onkeydown={(event) => handleKey(event, result.input_case_id)}><td>{result.input_case_id}</td>{#each selectedFeatures as feature}<td>{formatValue(valuesByCase.get(result.input_case_id)?.[feature])}</td>{/each}<td>{formatScore(result.score)}</td><td>{formatDuration(result.execution_time_ns)}</td><td>{result.status === 'succeeded' ? 'OK' : statusLabel(result.status)}</td></tr>{/each}</tbody>
        </table>
      </div>
    {/if}
  {/if}
</section>

<style>
  .analysis-panel { margin-bottom: 20px; }
  .analysis-panel > :not(.section-heading) { background: var(--paper); }
  .conditions { display: grid; gap: 8px; padding: 16px; }
  .condition { display: grid; grid-template-columns: 24px 1fr 90px 140px 48px; align-items: center; gap: 8px; }
  .condition > span { color: var(--pencil); font: 11px var(--mono); text-align: center; }
  .analysis-summary { display: grid; grid-template-columns: repeat(4, 1fr); margin: 0; border-bottom: 1px solid var(--rule); }
  .analysis-summary div { padding: 14px 16px; border-right: 1px solid var(--rule); }
  .analysis-summary dt, .analysis-table th { color: var(--pencil); font: 11px var(--mono); }
  .analysis-summary dd { margin: 7px 0 0; color: var(--graphite); font: 17px var(--mono); }
  .analysis-table { max-height: 320px; overflow: auto; }
  .analysis-table table { width: 100%; border-collapse: collapse; }
  .analysis-table th, .analysis-table td { padding: 9px 12px; text-align: left; border-bottom: 1px solid var(--rule); }
  .analysis-table td { color: var(--graphite-soft); font: 12px var(--mono); }
  .analysis-table tr[role='button'] { cursor: pointer; }
  .analysis-table tr[role='button']:hover, .analysis-table tr[role='button']:focus { background: var(--paper-shade); outline: none; }
</style>

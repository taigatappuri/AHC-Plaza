<script lang="ts">
  import { onMount } from 'svelte'
  import { errorMessage, requestJSON } from '../lib/api'
  import { cloneConfig, validateConfig } from '../lib/config-form'
  import type { ConfigData } from '../lib/types'

  export let configData: ConfigData | null = null
  export let onSave: (config: ConfigData) => Promise<ConfigData> = async (config) => config

  let form: ConfigData | null = null
  let sourceConfig: ConfigData | null = null
  let baseline = ''
  let saving = false
  let feedback = ''
  let validationErrors: Record<string, string> = {}
  let featureSources: string[] = []
  let featureSourcesError = ''

  $: if (configData && configData !== sourceConfig) {
    sourceConfig = configData
    form = cloneConfig(configData)
    baseline = JSON.stringify(configData)
    feedback = ''
  }
  $: dirty = form !== null && JSON.stringify(form) !== baseline
  $: validationErrors = form ? validateConfig(form) : {}
  $: errorCount = Object.keys(validationErrors).length

  onMount(() => {
    const handleKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 's' && dirty) {
        event.preventDefault()
        void save()
      }
    }
    const handleUnload = (event: BeforeUnloadEvent) => {
      if (!dirty) return
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('keydown', handleKey)
    window.addEventListener('beforeunload', handleUnload)
    void loadFeatureSources()
    return () => {
      window.removeEventListener('keydown', handleKey)
      window.removeEventListener('beforeunload', handleUnload)
    }
  })

  async function loadFeatureSources() {
    featureSourcesError = ''
    try {
      featureSources = (await requestJSON<{ sources: string[] }>('/api/input-features')).sources ?? []
    } catch (reason) {
      featureSourcesError = errorMessage(reason)
    }
  }

  function addVariable() {
    if (!form || form.input_format.variables.length >= 256) return
    const previous = form.input_format.variables.at(-1)
    form.input_format.variables = [...form.input_format.variables, { name: '', line: previous?.line ?? 1, column: (previous?.column ?? 0) + 1 }]
  }

  function addFeature() {
    if (!form || form.input_format.features.length >= 32 || !featureSources[0]) return
    const source = featureSources.find((item) => !form?.input_format.features.some((feature) => feature.source === item)) ?? featureSources[0]
    const stem = source.split('/').pop()?.replace(/\.cpp$/i, '') || 'feature'
    const names = new Set([...form.input_format.variables, ...form.input_format.features].map((item) => item.name))
    let name = stem
    for (let suffix = 2; names.has(name); suffix += 1) name = `${stem}_${suffix}`
    form.input_format.features = [...form.input_format.features, { name, source, timeout_ms: 2000 }]
  }

  function reset() {
    if (baseline) form = JSON.parse(baseline) as ConfigData
    feedback = ''
  }

  async function save() {
    if (!form || saving || errorCount) return
    saving = true
    feedback = ''
    try {
      const saved = await onSave(cloneConfig(form))
      sourceConfig = saved
      configData = saved
      form = cloneConfig(saved)
      baseline = JSON.stringify(saved)
      feedback = '保存しました'
    } catch (reason) {
      feedback = errorMessage(reason)
    } finally {
      saving = false
    }
  }
</script>

{#if !form}
  <div class="empty-state compact"><strong>設定を読み込み中…</strong></div>
{:else}
  <form class="settings-form" onsubmit={(event) => { event.preventDefault(); void save() }}>
    <header class="settings-status">
      <span>{errorCount ? `${errorCount}件の入力を確認` : feedback || (dirty ? '未保存の変更あり' : '')}</span>
      <div><button type="button" class="text-action" onclick={reset} disabled={!dirty}>元に戻す</button><button class="primary-action" disabled={!dirty || Boolean(errorCount) || saving}>{saving ? '保存中…' : '保存'}</button></div>
    </header>

    <section class="settings-section">
      <div class="settings-section-heading"><h3>プロジェクト</h3><p></p></div>
      <div class="settings-section-body">
        <div class="form-grid">
          <label>プロジェクト名<input bind:value={form.project.problem} class:invalid={validationErrors['project.problem']} /></label>
          <label>目的<select bind:value={form.project.objective}><option value="max">最大化</option><option value="min">最小化</option></select></label>
        </div>
      </div>
    </section>

    <section class="settings-section">
      <div class="settings-section-heading"><h3>ディレクトリ</h3><p>プロジェクトディレクトリからの相対パス</p></div>
      <div class="settings-section-body">
        <div class="form-grid">
          <label>Solver<input bind:value={form.paths.solver_dir} class:invalid={validationErrors['paths.solver_dir']} /></label>
          <label>tools<input bind:value={form.paths.tools_dir} class:invalid={validationErrors['paths.tools_dir']} /></label>
          <label>inputs<input bind:value={form.execution.default_input_dir} class:invalid={validationErrors['execution.default_input_dir']} /></label>
        </div>
      </div>
    </section>

    <section class="settings-section">
      <div class="settings-section-heading"><h3>実行</h3><p></p></div>
      <div class="settings-section-body">
        <div class="form-grid">
          <label>スレッド数<input type="number" min="0" bind:value={form.execution.threads} class:invalid={validationErrors['execution.threads']} /></label>
          <label>全ケースのタイムアウト（秒）<input type="number" min="1" bind:value={form.execution.timeout_seconds} class:invalid={validationErrors['execution.timeout_seconds']} /></label>
          <label>pahcer設定ファイル<input bind:value={form.pahcer.setting_file} class:invalid={validationErrors['pahcer.setting_file']} /></label>
        </div>
      </div>
    </section>

    <section class="settings-section">
      <div class="settings-section-heading"><h3>スコアと統計</h3><p></p></div>
      <div class="settings-section-body">
        <div class="form-grid">
          <label>無効スコア<input type="number" step="any" bind:value={form.score.invalid_score} class:invalid={validationErrors['score.invalid_score']} /></label>
          <label>信頼水準<input type="number" min="0.01" max="0.99" step="0.01" bind:value={form.statistics.confidence_level} class:invalid={validationErrors['statistics.confidence_level']} /></label>
          <label>ブートストラップ回数<input type="number" min="1" bind:value={form.statistics.bootstrap_iterations} class:invalid={validationErrors['statistics.bootstrap_iterations']} /></label>
          <label class="checkbox"><input type="checkbox" bind:checked={form.score.include_invalid_cases} />無効ケースを含める</label>
        </div>
      </div>
    </section>

    <section class="settings-section" id="config-input-format">
      <div class="settings-section-heading"><h3>入力特徴量</h3><p></p></div>
      <div class="settings-section-body">
        <div class="section-title"><h4>位置から指定</h4><button type="button" class="text-action" onclick={addVariable}>項目を追加</button></div>
        <div class="item-list">
          {#each form.input_format.variables as variable, index}
            <div class="variable-row">
              <input type="number" min="1" bind:value={variable.line} class:invalid={validationErrors[`input_format.variables.${index}.line`] || validationErrors[`input_format.variables.${index}.position`]} aria-label={`変数${index + 1}の行`} />
              <input type="number" min="1" bind:value={variable.column} class:invalid={validationErrors[`input_format.variables.${index}.column`] || validationErrors[`input_format.variables.${index}.position`]} aria-label={`変数${index + 1}の列`} />
              <input bind:value={variable.name} class:invalid={validationErrors[`input_format.variables.${index}.name`]} placeholder="変数名" aria-label={`変数${index + 1}の名前`} />
              <button class="remove-action" type="button" onclick={() => form && (form.input_format.variables = form.input_format.variables.filter((_, itemIndex) => itemIndex !== index))}>削除</button>
            </div>
          {/each}
        </div>

        <div class="section-title feature-title"><h4>C++から取得</h4><div><button type="button" class="text-action" onclick={loadFeatureSources}>ソースを再読込</button><button type="button" class="text-action" onclick={addFeature} disabled={!featureSources.length}>項目を追加</button></div></div>
        {#if featureSourcesError}<p class="form-error">{featureSourcesError}</p>{/if}
        <div class="item-list">
          {#each form.input_format.features as feature, index}
            <div class="feature-row">
              <input bind:value={feature.name} class:invalid={validationErrors[`input_format.features.${index}.name`]} placeholder="名前" aria-label={`派生特徴量${index + 1}の名前`} />
              <select bind:value={feature.source} class:invalid={validationErrors[`input_format.features.${index}.source`]} aria-label={`派生特徴量${index + 1}のソース`}>{#if !featureSources.includes(feature.source)}<option value={feature.source}>{feature.source}</option>{/if}{#each featureSources as source}<option value={source}>{source}</option>{/each}</select>
              <input type="number" min="1" max="60000" bind:value={feature.timeout_ms} class:invalid={validationErrors[`input_format.features.${index}.timeout_ms`]} aria-label={`派生特徴量${index + 1}のタイムアウト`} />
              <button class="remove-action" type="button" onclick={() => form && (form.input_format.features = form.input_format.features.filter((_, itemIndex) => itemIndex !== index))}>削除</button>
            </div>
          {/each}
        </div>
      </div>
    </section>
  </form>
{/if}

<style>
  .settings-form { width: 100%; overflow: visible; }
  .settings-status, .section-title { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
  .settings-status { position: sticky; z-index: 2; top: 52px; min-height: 52px; margin-bottom: 4px; padding: 8px 0; background: color-mix(in srgb, var(--bench) 94%, transparent); backdrop-filter: blur(10px); }
  .settings-status > span { color: var(--pencil); font-size: 12px; }
  .settings-status > div, .section-title > div { display: flex; gap: 12px; }
  .settings-section { display: grid; grid-template-columns: 180px minmax(0, 1fr); gap: 32px; padding: 28px 0; border-bottom: 1px solid var(--rule); scroll-margin-top: 116px; }
  .settings-section:last-child { border-bottom: 0; }
  .settings-section-heading h3 { margin: 0; color: var(--graphite); font-size: 16px; }
  .settings-section-heading p { margin: 7px 0 0; color: var(--pencil); font-size: 11px; line-height: 1.6; }
  .settings-section-body { min-width: 0; }
  .section-title h4 { margin: 0; color: var(--graphite); font-size: 13px; }
  .form-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 16px; }
  .form-grid label { display: grid; gap: 7px; color: var(--graphite-soft); font-size: 12px; }
  .checkbox { display: flex !important; align-items: center; }
  .checkbox input { width: 16px; min-height: 16px; }
  .item-list { display: grid; gap: 8px; margin-top: 12px; }
  .variable-row, .feature-row { display: grid; gap: 8px; }
  .variable-row { grid-template-columns: 90px 90px 1fr 48px; }
  .feature-row { grid-template-columns: 1fr 1.5fr 120px 48px; }
  .feature-title { margin-top: 28px; }
  .invalid { border-color: var(--failure); }
  .form-error { color: var(--failure); font-size: 11px; }

  @media (max-width: 1050px) {
    .settings-section { grid-template-columns: 150px minmax(0, 1fr); gap: 24px; }
  }
</style>

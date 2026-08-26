<script lang="ts">
  import { tick } from 'svelte'
  import AppHeader from './components/AppHeader.svelte'
  import ComparePage from './components/ComparePage.svelte'
  import InputCreatePage from './components/InputCreatePage.svelte'
  import RunCommand from './components/RunCommand.svelte'
  import RunDetail from './components/RunDetail.svelte'
  import RunTable from './components/RunTable.svelte'
  import SettingsPage from './components/SettingsPage.svelte'
  import { errorMessage, requestJSON, requestText } from './lib/api'
  import { shortId, statusLabel } from './lib/formatters'
  import type { CaseResult, Comparison, ConfigData, InputGenerateRequest, InputGenerateResult, InputGenerator, InputFeatureDefinition, Run, RunStatistics, Tab, TabDefinition, FeatureCondition, FeatureData } from './lib/types'

  type RunDetailResponse = {
    cases: CaseResult[]
    statistics: RunStatistics
    feature_data: FeatureData
    result?: { error?: string } | null
  }
  type RunLogs = { stdout: string; stderr: string }
  type RunStatusEvent = Partial<Pick<Run, 'run_number' | 'created_at'>> & {
    status: string
    error?: string
  }

  const applyRunStatus = (run: Run, status: RunStatusEvent): Run => ({
    ...run,
    status: status.status,
    run_number: status.run_number ?? run.run_number,
    created_at: status.created_at ?? run.created_at
  })

  let activeTab: Tab = 'overview'
  let runs: Run[] = []
  let runsRefreshing = false
  let selectedRun: Run | null = null
  let caseResults: CaseResult[] = []
  let runStatistics: RunStatistics | null = null
  let featureData: FeatureData | null = null
  let source = ''
  let logs = { stdout: '', stderr: '' }
  let runError = ''
  let configData: ConfigData | null = null
  let analysisFeatures: InputFeatureDefinition[] = []
  let comparison: Comparison | null = null
  let comparing = false
  let message = ''
  let loading = false
  let solver = 'solver/main.cpp'
  let solvers: string[] = []
  let solverLoading = false
  let solverError = ''
  let inputDirectories: string[] = []
  let inputDirectoryLoading = false
  let inputDirectoryError = ''
  let inputDir = ''
  let inputGenerators: InputGenerator[] = []
  let inputGeneratorLoading = false
  let inputGeneratorError = ''
  let inputGenerationResult: InputGenerateResult | null = null
  let inputGenerating = false
  let threads = 0
  let timeoutSeconds = 300
  let comment = ''
  let compareA = ''
  let compareB = ''
  let eventSource: EventSource | null = null
  let lastLogRefresh = 0
  let logRefreshInFlight = false
  let runQuery = ''
  let showingRunDetail = false

  const tabs: TabDefinition[] = [
    { id: 'overview', label: '実行', hint: '実行を開始' },
    { id: 'detail', label: '履歴', hint: 'Runの履歴を確認' },
    { id: 'compare', label: '比較', hint: 'Runを比較' },
    { id: 'input', label: '入力', hint: '入力ケースを生成' },
    { id: 'settings', label: '設定', hint: 'プロジェクト設定を変更' }
  ]

  $: currentPageTitle = ({
    overview: '実行',
    detail: showingRunDetail ? '実行の詳細' : '履歴',
    compare: '比較',
    input: '入力生成',
    settings: '設定'
  })[activeTab]

  $: analysisFeatures = configData ? [
    ...configData.input_format.variables.map((variable) => ({ ...variable, kind: 'position' as const })),
    ...configData.input_format.features.map((feature) => ({ name: feature.name, kind: 'cpp' as const, source: feature.source }))
  ] : []

  async function loadRuns() {
    runs = await requestJSON<Run[]>('/api/runs')
    if (!compareA && runs[0]) compareA = runs[0].id
    if (!compareB && runs[1]) compareB = runs[1].id
    if (selectedRun) {
      const latest = runs.find((run) => run.id === selectedRun?.id)
      if (latest) selectedRun = latest
    }
  }

  async function refreshRuns() {
    if (runsRefreshing) return
    runsRefreshing = true
    message = ''
    try {
      await loadRuns()
    } catch (error) {
      message = errorMessage(error)
    } finally {
      runsRefreshing = false
    }
  }

  async function loadConfig() {
    configData = await requestJSON<ConfigData>('/api/config')
    resetRunDefaults()
  }

  function resetRunDefaults() {
    if (!configData) return
    threads = configData.execution.threads
    timeoutSeconds = configData.execution.timeout_seconds
  }

  async function loadSolvers() {
    solverLoading = true
    solverError = ''
    try {
      const response = await requestJSON<{ solvers: string[] }>('/api/solvers')
      solvers = response.solvers
      if (!solvers.includes(solver)) solver = solvers[0] ?? ''
    } catch (error) {
      solvers = []
      solver = ''
      solverError = errorMessage(error)
    } finally {
      solverLoading = false
    }
  }

  async function loadInputDirectories() {
    inputDirectoryLoading = true
    inputDirectoryError = ''
    try {
      const response = await requestJSON<{ input_directories: string[] }>('/api/input-directories')
      inputDirectories = response.input_directories
      if (!inputDirectories.includes(inputDir)) inputDir = inputDirectories[0] ?? ''
    } catch (error) {
      inputDirectories = []
      inputDirectoryError = errorMessage(error)
    } finally {
      inputDirectoryLoading = false
    }
  }

  async function loadInputGenerators() {
    inputGeneratorLoading = true
    inputGeneratorError = ''
    try {
      const response = await requestJSON<{ generators: InputGenerator[] }>('/api/input-generators')
      inputGenerators = response.generators
    } catch (error) {
      inputGenerators = []
      inputGeneratorError = errorMessage(error)
    } finally {
      inputGeneratorLoading = false
    }
  }

  async function generateInputCases(input: InputGenerateRequest) {
    inputGenerating = true
    inputGenerationResult = null
    inputGeneratorError = ''
    try {
      const result = await requestJSON<InputGenerateResult>('/api/input-generate-tool', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input)
      })
      inputGenerationResult = result
      await loadInputDirectories()
      inputDir = result.output_dir
      message = `入力を${result.case_count}件生成`
    } catch (error) {
      inputGeneratorError = errorMessage(error)
    } finally {
      inputGenerating = false
    }
  }

  async function saveConfig(config: ConfigData) {
    const saved = await requestJSON<ConfigData>('/api/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(config)
    })
    configData = saved
    await loadSolvers()
    await loadInputDirectories()
    return saved
  }

  async function selectRun(run: Run, navigate = true) {
    selectedRun = run
    runStatistics = null
    featureData = null
    source = ''
    runError = ''
    const encodedRunID = encodeURIComponent(run.id)
    const detail = await requestJSON<RunDetailResponse>(`/api/runs/${encodedRunID}`)
    caseResults = detail.cases
    runStatistics = detail.statistics
    featureData = detail.feature_data
    runError = detail.result?.error ?? ''
    source = await requestText(`/api/runs/${encodedRunID}/source`).catch(() => '')
    logs = await requestJSON<RunLogs>(`/api/runs/${encodedRunID}/logs`)
    if (navigate) activeTab = 'detail'
  }

  async function openRunDetail(run: Run) {
    try {
      await selectRun(run, false)
      showingRunDetail = true
      activeTab = 'detail'
    } catch (error) {
      message = errorMessage(error)
    }
  }

  async function refreshLogs(runId: string) {
    const now = Date.now()
    if (logRefreshInFlight || now - lastLogRefresh < 1000) return
    logRefreshInFlight = true
    lastLogRefresh = now
    try {
      if (selectedRun?.id === runId) logs = await requestJSON<RunLogs>(`/api/runs/${encodeURIComponent(runId)}/logs`)
    } finally {
      logRefreshInFlight = false
    }
  }

  async function startRun() {
    loading = true
    message = ''
    try {
      const result = await requestJSON<{ run_id: string; status: string }>('/api/runs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ solver, input_dir: inputDir, threads, timeout_seconds: timeoutSeconds, comment })
      })
      message = `Run ${shortId(result.run_id)} を開始`
      await loadRuns()
      eventSource?.close()
      eventSource = new EventSource(`/api/runs/${result.run_id}/events`)
      eventSource.addEventListener('status', async (event) => {
        const status = JSON.parse((event as MessageEvent).data) as RunStatusEvent
        message = status.error
          ? `Run ${shortId(result.run_id)}: ${status.error}`
          : `Run ${shortId(result.run_id)}: ${statusLabel(status.status)}`
        if (status.error && selectedRun?.id === result.run_id) runError = status.error
        runs = runs.map((item) => item.id === result.run_id ? applyRunStatus(item, status) : item)
        if (selectedRun && selectedRun.id === result.run_id) {
          selectedRun = applyRunStatus(selectedRun, status)
        }
        await refreshLogs(result.run_id)
        if (['succeeded', 'partial', 'failed', 'cancelled'].includes(status.status)) {
          await loadRuns()
          const current = runs.find((item) => item.id === result.run_id)
          if (current) await selectRun(current, false)
          eventSource?.close()
          eventSource = null
          loading = false
        }
      })
      eventSource.onerror = () => {
        eventSource?.close()
        eventSource = null
        loading = false
      }
    } catch (error) {
      message = errorMessage(error)
      loading = false
    }
  }

  async function cancelRun() {
    if (!selectedRun) return
    try {
      await requestJSON<{ run_id: string; status: string }>(`/api/runs/${encodeURIComponent(selectedRun.id)}/cancel`, { method: 'POST' })
      message = `停止要求: ${shortId(selectedRun.id)}`
    } catch (error) {
      message = errorMessage(error)
    }
  }

  async function updateRunComment(comment: string) {
    if (!selectedRun) return
    const updated = await requestJSON<Run>(`/api/runs/${encodeURIComponent(selectedRun.id)}/comment`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ comment })
    })
    selectedRun = updated
    runs = runs.map((run) => run.id === updated.id ? { ...run, comment: updated.comment } : run)
    message = 'コメントを更新しました'
  }

  async function configureInputFormat() {
    activeTab = 'settings'
    await tick()
    document.getElementById('config-input-format')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }

  async function compareRuns(runA = compareA, runB = compareB, conditions: FeatureCondition[] = []) {
    comparison = null
    comparing = true
    try {
      comparison = await requestJSON<Comparison>('/api/compare', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ run_a: runA, run_b: runB, conditions })
      })
    } catch (error) {
      message = errorMessage(error)
    } finally {
      comparing = false
    }
  }

  void loadRuns().catch((error) => { message = errorMessage(error) })
  void loadConfig().catch((error) => { message = errorMessage(error) })
  void loadSolvers().catch((error) => { solverError = errorMessage(error) })
  void loadInputDirectories().catch((error) => { inputDirectoryError = errorMessage(error) })
  void loadInputGenerators()
</script>

<div class="app-shell">
  <AppHeader
    {tabs}
    activeTab={activeTab}
    problem={configData?.project.problem ?? ''}
    onNavigate={(tab) => { activeTab = tab; if (tab === 'detail') showingRunDetail = false }}
  />

  <main class="main-content">
    <header class="page-heading">
      <h1>{currentPageTitle}</h1>
      {#if activeTab === 'compare' || (activeTab === 'detail' && !showingRunDetail)}
        <button class="context-action" type="button" onclick={refreshRuns} disabled={runsRefreshing}>{runsRefreshing ? '更新中…' : '更新'}</button>
      {/if}
    </header>

    <div>
      {#if message}<div class="notice">{message}</div>{/if}

      {#if activeTab === 'overview'}
        <RunCommand bind:solver {solvers} {solverLoading} {solverError} onRefreshSolvers={loadSolvers} {inputDirectories} inputRoot={configData?.execution.default_input_dir ?? 'ahc-plaza/inputs'} {inputDirectoryLoading} {inputDirectoryError} onRefreshInputDirectories={loadInputDirectories} bind:inputDir bind:threads bind:timeoutSeconds bind:comment {loading} defaultsAvailable={configData !== null} onResetDefaults={resetRunDefaults} onSubmit={startRun} />
        <section class="recent-runs">
          <header class="section-heading"><h2>直近の実行履歴</h2><button class="context-action" type="button" onclick={refreshRuns} disabled={runsRefreshing}>{runsRefreshing ? '更新中…' : '更新'}</button></header>
          <RunTable runs={runs.slice(0, 5)} selectedRunId={selectedRun?.id ?? ''} compact onSelect={openRunDetail} />
        </section>

      {:else if activeTab === 'input'}
        <InputCreatePage generators={inputGenerators} {inputGeneratorLoading} error={inputGeneratorError} result={inputGenerationResult} defaultOutputDir={configData?.execution.default_input_dir ?? 'ahc-plaza/inputs'} loading={inputGenerating} onGenerate={generateInputCases} />
      {:else if activeTab === 'detail'}
        {#if showingRunDetail}
          <RunDetail {selectedRun} {caseResults} {runStatistics} {featureData} {source} {logs} {runError} onCancel={cancelRun} onBack={() => showingRunDetail = false} onUpdateComment={updateRunComment} onConfigureInputFormat={configureInputFormat} />
        {:else}
          <RunTable bind:query={runQuery} {runs} selectedRunId={selectedRun?.id ?? ''} onSelect={openRunDetail} />
        {/if}
      {:else if activeTab === 'compare'}
        <ComparePage bind:compareA bind:compareB {runs} {comparison} features={analysisFeatures} objective={configData?.project.objective ?? 'max'} loading={comparing} onCompare={compareRuns} onInvalidate={() => comparison = null} onConfigureInputFormat={configureInputFormat} />
      {:else}
        <SettingsPage {configData} onSave={saveConfig} />
      {/if}
    </div>
  </main>
</div>

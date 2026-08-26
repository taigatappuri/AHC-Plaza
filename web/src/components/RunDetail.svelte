<script lang="ts">
  import { onDestroy, tick } from 'svelte'
  import { errorMessage } from '../lib/api'
  import { directoryName, formatDuration, formatScore, formatTime, runLabel, statusLabel } from '../lib/formatters'
  import type { CaseResult, Run, RunStatistics, FeatureData } from '../lib/types'
  import ScoreDistributionChart from './ScoreDistributionChart.svelte'
  import FeatureScoreAnalysis from './FeatureScoreAnalysis.svelte'
  import VisualizerPanel from './VisualizerPanel.svelte'

  export let selectedRun: Run | null = null
  export let runStatistics: RunStatistics | null = null
  export let featureData: FeatureData | null = null
  export let caseResults: CaseResult[] = []
  export let source = ''
  export let logs = { stdout: '', stderr: '' }
  export let runError = ''
  export let onCancel: () => void = () => {}
  export let onBack: () => void = () => {}
  export let onUpdateComment: (comment: string) => Promise<void> = async () => {}
  export let onConfigureInputFormat: () => void = () => {}

  $: displayedStatistics = runStatistics ?? selectedRun
  $: hasStatistics = (displayedStatistics?.case_count ?? 0) > 0
  const duration = (value: number | undefined) => value == null ? '—' : formatDuration(value)

  let copyingSnapshot = false
  let snapshotCopied = false
  let snapshotCopyError = ''
  let snapshotCopyResetTimer: ReturnType<typeof setTimeout> | undefined
  let selectedCaseID = ''
  let visualizerAnchor: HTMLElement | undefined
  let activeArtifact: 'logs' | 'source' = 'logs'
  let editingComment = false
  let commentDraft = ''
  let commentSaving = false
  let commentError = ''
  type CaseSortKey = 'name' | 'score' | 'time'
  type SortDirection = 'asc' | 'desc'
  type AriaSort = 'ascending' | 'descending' | 'none'

  let caseSortKey: CaseSortKey | null = null
  let caseSortDirection: SortDirection = 'asc'
  const caseSortLabels: Record<CaseSortKey, string> = { name: 'ケース名', score: 'スコア', time: '時間' }

  $: displayedCaseResults = caseSortKey === null ? caseResults : [...caseResults].sort((left, right) => {
    const comparison = caseSortKey === 'name'
      ? left.input_case_id.localeCompare(right.input_case_id, undefined, { numeric: true })
      : caseSortKey === 'score'
        ? left.score - right.score
        : left.execution_time_ns - right.execution_time_ns
    return caseSortDirection === 'asc' ? comparison : -comparison
  })

  function sortCases(key: CaseSortKey) {
    if (caseSortKey === key) {
      caseSortDirection = caseSortDirection === 'asc' ? 'desc' : 'asc'
      return
    }
    caseSortKey = key
    caseSortDirection = 'asc'
  }

  function caseSortIndicator(key: CaseSortKey) {
    if (caseSortKey !== key) return '↕'
    return caseSortDirection === 'asc' ? '↑' : '↓'
  }

  function caseSortAriaValue(key: CaseSortKey): AriaSort {
    if (caseSortKey !== key) return 'none'
    return caseSortDirection === 'asc' ? 'ascending' : 'descending'
  }

  function nextCaseSortDirection(key: CaseSortKey) {
    return caseSortKey === key && caseSortDirection === 'asc' ? '降順' : '昇順'
  }

  $: if (selectedRun && !editingComment) commentDraft = selectedRun.comment

  onDestroy(() => {
    if (snapshotCopyResetTimer) clearTimeout(snapshotCopyResetTimer)
  })

  async function selectCase(caseID: string) {
    selectedCaseID = caseID
    await tick()
    visualizerAnchor?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }

  function handleCaseRowKeydown(event: KeyboardEvent, caseID: string) {
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    void selectCase(caseID)
  }

  async function copySnapshot() {
    if (!source || copyingSnapshot) return

    copyingSnapshot = true
    snapshotCopied = false
    snapshotCopyError = ''
    try {
      await navigator.clipboard.writeText(source)
      snapshotCopied = true
      if (snapshotCopyResetTimer) clearTimeout(snapshotCopyResetTimer)
      snapshotCopyResetTimer = setTimeout(() => {
        snapshotCopied = false
      }, 1800)
    } catch (reason) {
      snapshotCopyError = errorMessage(reason)
    } finally {
      copyingSnapshot = false
    }
  }

  async function saveComment() {
    commentSaving = true
    commentError = ''
    try {
      await onUpdateComment(commentDraft)
      editingComment = false
    } catch (reason) {
      commentError = errorMessage(reason)
    } finally {
      commentSaving = false
    }
  }
</script>

<section class="detail-toolbar">
  <div>
    {#if selectedRun}
      <div class="run-title"><h2>{runLabel(selectedRun.run_number, selectedRun.id)}</h2><span class="run-status-badge {selectedRun.status}">{statusLabel(selectedRun.status)}</span></div>
      <p>{selectedRun.solver_path} / {directoryName(selectedRun.input_dir)} / {formatTime(selectedRun.created_at)}</p>
      {#if editingComment}
        <form class="comment-editor" onsubmit={(event) => { event.preventDefault(); void saveComment() }}>
          <input bind:value={commentDraft} aria-label="コメント" placeholder="コメント" disabled={commentSaving} />
          <button class="primary-action" disabled={commentSaving}>{commentSaving ? '保存中…' : '保存'}</button>
          <button class="text-action" type="button" onclick={() => editingComment = false} disabled={commentSaving}>キャンセル</button>
        </form>
        {#if commentError}<small class="comment-error">{commentError}</small>{/if}
      {:else}
        <div class="run-note-row"><p class="run-note">{selectedRun.comment || 'コメントなし'}</p><button class="text-action" onclick={() => { commentError = ''; editingComment = true }}>編集</button></div>
      {/if}
    {:else}
      <h2>実行を選択</h2>
    {/if}
  </div>
  <div class="detail-toolbar-actions">
    <button class="back-action" onclick={onBack}>← 履歴</button>
    {#if selectedRun && (selectedRun.status === 'running' || selectedRun.status === 'queued')}<button class="danger-action" onclick={onCancel}>停止</button>{/if}
  </div>
</section>

{#if selectedRun}
  {#if runError}<div class="error-banner"><strong>実行失敗</strong><span>{runError}</span></div>{/if}
  <section class="run-overview">
    <div class="section-heading">
      <h2>統計</h2>
      <div class="stats-directory"><span>入力</span><strong>{directoryName(selectedRun.input_dir)}</strong><small>{selectedRun.input_dir}</small></div>
    </div>
    <dl class="run-summary">
      <div><dt>平均</dt><dd>{hasStatistics ? formatScore(displayedStatistics?.average_score) : '—'}</dd></div>
      <div><dt>中央値</dt><dd>{hasStatistics ? formatScore(displayedStatistics?.median_score) : '—'}</dd></div>
      <div><dt>標準偏差</dt><dd>{hasStatistics ? formatScore(displayedStatistics?.stddev_score) : '—'}</dd></div>
      <div><dt>平均時間</dt><dd>{hasStatistics ? duration(displayedStatistics?.average_execution_time_ns) : '—'}</dd></div>
      <div><dt>ケース</dt><dd>{hasStatistics ? displayedStatistics?.case_count : '—'}</dd></div>
    </dl>
  </section>
  <ScoreDistributionChart {caseResults} />
  <FeatureScoreAnalysis {caseResults} {featureData} onSelectCase={selectCase} {onConfigureInputFormat} />
  <div class="detail-grid">
    <div class="panel case-panel"><div class="panel-header"><h2>ケース</h2><div class="case-panel-meta"><span class="count-badge">{caseResults.length}件</span></div></div><div class="case-table-wrap"><table><thead><tr><th aria-sort={caseSortAriaValue('name')}><button class="case-sort-button" type="button" aria-label={`${caseSortLabels.name}を${nextCaseSortDirection('name')}でソート`} onclick={() => sortCases('name')}>{caseSortLabels.name}<span class="case-sort-indicator" aria-hidden="true">{caseSortIndicator('name')}</span></button></th><th aria-sort={caseSortAriaValue('score')}><button class="case-sort-button" type="button" aria-label={`${caseSortLabels.score}を${nextCaseSortDirection('score')}でソート`} onclick={() => sortCases('score')}>{caseSortLabels.score}<span class="case-sort-indicator" aria-hidden="true">{caseSortIndicator('score')}</span></button></th><th aria-sort={caseSortAriaValue('time')}><button class="case-sort-button" type="button" aria-label={`${caseSortLabels.time}を${nextCaseSortDirection('time')}でソート`} onclick={() => sortCases('time')}>{caseSortLabels.time}<span class="case-sort-indicator" aria-hidden="true">{caseSortIndicator('time')}</span></button></th><th>状態</th></tr></thead><tbody>{#each displayedCaseResults as item}<tr class="case-row" class:selected={selectedCaseID === item.input_case_id} role="button" tabindex="0" aria-pressed={selectedCaseID === item.input_case_id} title="ビジュアライザでケースを表示" onclick={() => selectCase(item.input_case_id)} onkeydown={(event) => handleCaseRowKeydown(event, item.input_case_id)}><td class="mono">{item.input_case_id}</td><td class="score-cell">{item.score.toLocaleString()}</td><td class="mono muted-text">{formatDuration(item.execution_time_ns)}</td><td><span class="case-status {item.status}">{item.status === 'succeeded' ? 'OK' : statusLabel(item.status)}</span></td></tr>{/each}</tbody></table></div></div>
    <div class="visualizer-anchor" bind:this={visualizerAnchor}><VisualizerPanel {selectedRun} {caseResults} bind:selectedCaseID /></div>
  </div>
  <section class="artifact-panel">
    <header class="artifact-header">
      <div class="artifact-tabs" role="tablist" aria-label="実行の成果物">
        <button class:active={activeArtifact === 'logs'} role="tab" aria-selected={activeArtifact === 'logs'} onclick={() => activeArtifact = 'logs'}>ログ</button>
        <button class:active={activeArtifact === 'source'} role="tab" aria-selected={activeArtifact === 'source'} onclick={() => activeArtifact = 'source'}>ソース</button>
      </div>
      {#if activeArtifact === 'logs'}
        <span class="log-label">stdout / stderr</span>
      {:else}
        <div class="source-header-actions"><button class:copied={snapshotCopied} class="copy-action" disabled={!source || copyingSnapshot} onclick={copySnapshot} aria-label="スナップショットをコピー">{copyingSnapshot ? 'コピー中…' : snapshotCopied ? 'コピー済み' : 'コピー'}</button><span class="hash-label">{selectedRun.source_hash?.slice(0, 8) ?? 'snapshot'}</span></div>
      {/if}
    </header>
    {#if activeArtifact === 'logs'}
      <pre class="log-output">{logs.stdout}{logs.stderr ? `\n--- stderr ---\n${logs.stderr}` : ''}</pre>
    {:else}
      {#if snapshotCopyError}<div class="source-copy-error">コピーできませんでした</div>{/if}
      <pre class="source-code">{source || 'ソースを読み込み中…'}</pre>
    {/if}
  </section>
{:else}
  <div class="empty-state page-empty"><strong>履歴から実行を選択してください</strong></div>
{/if}

<style>
  .run-title { display: flex; align-items: center; gap: 10px; }
  .run-status-badge { padding: 3px 7px; color: var(--pencil); background: var(--paper-shade); border-radius: 3px; font: 11px var(--mono); }
  .back-action { padding: 2px 0; color: var(--graphite); background: transparent; font-size: 12px; font-weight: 500; }
  .back-action:hover { color: var(--selection); }
  .run-status-badge.succeeded { color: var(--pass); }
  .run-status-badge.failed { color: var(--failure); }
  .run-status-badge.running, .run-status-badge.queued { color: var(--selection); }
  .run-note-row, .comment-editor { display: flex; align-items: center; gap: 10px; margin-top: 8px; }
  .detail-toolbar .run-note { max-width: 760px; margin: 0; color: var(--graphite-soft); }
  .comment-editor input { width: min(520px, 50vw); min-height: 32px; padding-block: 5px; }
  .comment-editor .primary-action { min-height: 32px; }
  .comment-error { display: block; margin-top: 5px; color: var(--failure); font-size: 11px; }
  .run-overview { margin-bottom: 20px; }
  .stats-directory { display: flex; align-items: baseline; gap: 8px; min-width: 0; font: 11px var(--mono); }
  .stats-directory span, .stats-directory small { color: var(--pencil); }
  .stats-directory small { overflow: hidden; max-width: 320px; text-overflow: ellipsis; white-space: nowrap; }
  .run-summary { display: grid; grid-template-columns: repeat(5, 1fr); margin: 0; background: var(--paper); }
  .run-summary div { padding: 14px 16px; border-right: 1px solid var(--rule); }
  .run-summary div:last-child { border-right: 0; }
  .run-summary dt { color: var(--pencil); font-size: 11px; }
  .run-summary dd { margin: 7px 0 0; color: var(--graphite); font: 18px var(--mono); }
  .artifact-panel { overflow: hidden; background: var(--paper); }
  .artifact-header { display: flex; align-items: center; justify-content: space-between; min-height: 46px; padding: 0 16px; border-bottom: 1px solid var(--rule); }
  .artifact-tabs { display: flex; align-self: stretch; gap: 4px; }
  .artifact-tabs button { position: relative; padding: 0 12px; color: var(--pencil); background: transparent; font-size: 12px; }
  .artifact-tabs button.active { color: var(--graphite); font-weight: 600; }
  .artifact-tabs button.active::after { position: absolute; right: 10px; bottom: -1px; left: 10px; height: 2px; background: var(--selection); content: ''; }
  .case-sort-button { display: inline-flex; align-items: center; gap: 5px; padding: 0; color: inherit; background: transparent; font: inherit; text-align: left; }
  .case-sort-button:hover { color: var(--selection); }
  .case-sort-indicator { color: var(--selection); }
</style>

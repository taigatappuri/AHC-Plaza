<script lang="ts">
  import type { Run } from '../lib/types'
  import { directoryName, formatScore, formatTime, runLabel, statusLabel } from '../lib/formatters'
  import ScoreFingerprint from './ScoreFingerprint.svelte'

  export let runs: Run[] = []
  export let selectedRunId = ''
  export let query = ''
  export let compact = false
  export let onSelect: (run: Run) => void = () => {}

  $: normalizedQuery = query.trim().toLowerCase()
  $: matchingRuns = runs.filter((run) => !normalizedQuery || [run.id, run.solver_path, run.input_dir, run.comment, run.status]
    .some((value) => value?.toLowerCase().includes(normalizedQuery)))
</script>

<section class:compact class="run-ledger">
  {#if !compact}
  <div class="table-toolbar">
    <div class="table-actions"><input class="run-search" bind:value={query} placeholder="Runを検索…" /><span class="count-badge">{matchingRuns.length} / {runs.length}</span></div>
  </div>
  {/if}
  {#if runs.length === 0}
    <div class="empty-state"><strong>まだ実行はありません</strong></div>
  {:else}
    <div class="run-table-scroll">
      <div class="run-table-head"><span>実行</span><span>ソースファイル / 入力</span><span>コメント</span><span>スコア分布</span><span>平均</span><span>ケース</span><span>状態</span></div>
      <div class="run-list">
        {#each matchingRuns as run}
          {@const hasScores = (run.case_count ?? 0) > 0}
          <button class:selected={selectedRunId === run.id} class="run-row" onclick={() => onSelect(run)}>
            <span class="run-primary"><strong>{runLabel(run.run_number, run.id)}</strong><small>{formatTime(run.created_at)}</small></span>
            <span class="run-setup"><strong>{run.solver_path}</strong><small>{directoryName(run.input_dir)}</small></span>
            <span class="run-comment">{run.comment || '—'}</span>
            <ScoreFingerprint {run} />
            <span class="run-score">{hasScores ? formatScore(run.average_score) : '—'}</span>
            <span class="run-cases">{run.case_count ?? '—'}</span>
            <span class="run-status {run.status}">{statusLabel(run.status)}</span>
          </button>
        {/each}
      </div>
    </div>
  {/if}
</section>

<style>
  .run-ledger { overflow: hidden; background: var(--paper); }
  .table-toolbar { display: flex; justify-content: flex-end; padding: 12px 0; background: var(--bench); }
  .table-actions { display: flex; align-items: center; gap: 10px; }
  .run-search { width: 220px; min-height: 32px; }
  .run-table-scroll { overflow-x: auto; }
  .run-table-head, .run-row {
    display: grid;
    grid-template-columns: 140px 200px minmax(120px, 1fr) 170px 110px 64px 76px;
    align-items: center;
    gap: 12px;
    min-width: 940px;
    padding: 10px 16px;
  }
  .run-table-head { color: var(--pencil); border-bottom: 1px solid var(--rule); font: 11px var(--mono); }
  .run-row { width: 100%; min-height: 58px; color: var(--graphite-soft); text-align: left; background: transparent; border-bottom: 1px solid var(--rule); }
  .run-row:last-child { border-bottom: 0; }
  .run-row:hover, .run-row.selected { background: var(--paper-shade); }
  .run-row.selected { box-shadow: inset 2px 0 var(--selection); }
  .run-primary strong, .run-setup strong { display: block; overflow: hidden; color: var(--graphite); font: 13px var(--mono); text-overflow: ellipsis; white-space: nowrap; }
  .run-primary small, .run-setup small { display: block; overflow: hidden; margin-top: 4px; color: var(--pencil); font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
  .run-comment { overflow: hidden; color: var(--graphite-soft); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
  .run-score { font: 13px var(--mono); }
  .run-cases, .run-status { font: 12px var(--mono); }
  .run-status.succeeded { color: var(--pass); }
  .run-status.failed { color: var(--failure); }
  .run-status.running, .run-status.queued { color: var(--selection); }
  .compact .run-table-head, .compact .run-row { min-width: 900px; }
</style>

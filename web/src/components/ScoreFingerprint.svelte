<script lang="ts">
  import { formatScore } from '../lib/formatters'
  import type { Run } from '../lib/types'

  export let run: Run

  const position = (value: number | undefined, min: number, max: number) => {
    if (value == null || !Number.isFinite(value)) return 50
    if (min === max) return 50
    return Math.max(0, Math.min(100, ((value - min) / (max - min)) * 100))
  }

  $: min = run.min_score ?? 0
  $: max = run.max_score ?? 0
  $: ready = (run.case_count ?? 0) > 0 && [min, run.q1_score, run.median_score, run.q3_score, max].every(Number.isFinite)
  $: q1 = position(run.q1_score, min, max)
  $: median = position(run.median_score, min, max)
  $: q3 = position(run.q3_score, min, max)
  $: description = ready
    ? `最小 ${formatScore(min)}、Q1 ${formatScore(run.q1_score)}、中央値 ${formatScore(run.median_score)}、Q3 ${formatScore(run.q3_score)}、最大 ${formatScore(max)}`
    : 'スコアなし'
</script>

<span class:empty={!ready} class="score-fingerprint" role="img" aria-label={description} title={description}>
  {#if ready}
    <i class="whisker"></i>
    <i class="quartile" style={`left: ${q1}%; width: ${Math.max(2, q3 - q1)}%;`}></i>
    <i class="median" style={`left: ${median}%;`}></i>
    <i class="cap cap-min"></i>
    <i class="cap cap-max"></i>
  {:else}
    <span>スコアなし</span>
  {/if}
</span>

<style>
  .score-fingerprint { position: relative; display: block; width: 100%; height: 24px; }
  .score-fingerprint i { position: absolute; display: block; }
  .whisker { top: 11px; right: 0; left: 0; height: 1px; background: var(--rule-strong); }
  .quartile { top: 7px; height: 9px; min-width: 3px; background: var(--selection-soft); border: 1px solid var(--selection); border-radius: 2px; }
  .median { top: 5px; width: 2px; height: 13px; margin-left: -1px; background: var(--selection); }
  .cap { top: 8px; width: 1px; height: 7px; background: var(--pencil); }
  .cap-min { left: 0; }
  .cap-max { right: 0; }
  .score-fingerprint.empty { display: flex; align-items: center; color: var(--pencil); font: 11px var(--mono); }
</style>

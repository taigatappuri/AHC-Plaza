<script lang="ts">
  import { formatScore } from '../lib/formatters'
  import type { CaseResult } from '../lib/types'

  export let caseResults: CaseResult[] = []

  type Bin = { start: number; end: number; count: number }
  type Distribution = { bins: Bin[]; min: number; max: number; mean: number }

  const chartWidth = 1000
  const plotLeft = 34
  const plotRight = 980
  const plotTop = 24
  const plotBottom = 178

  function buildDistribution(values: number[]): Distribution | null {
    if (!values.length) return null
    const min = values.reduce((current, value) => Math.min(current, value))
    const max = values.reduce((current, value) => Math.max(current, value))
    const mean = values.reduce((sum, value) => sum + value, 0) / values.length
    const padding = min === max ? Math.max(Math.abs(min) * .05, 1) : 0
    const domainMin = min - padding
    const domainMax = max + padding
    const binCount = Math.min(64, Math.max(16, Math.ceil(Math.sqrt(values.length) * 3)))
    const step = (domainMax - domainMin) / binCount
    const bins = Array.from({ length: binCount }, (_, index) => ({
      start: domainMin + step * index,
      end: domainMin + step * (index + 1),
      count: 0
    }))
    for (const value of values) bins[Math.min(binCount - 1, Math.floor((value - domainMin) / step))].count += 1
    return { bins, min, max, mean }
  }

  $: scores = caseResults.map((item) => item.score).filter(Number.isFinite)
  $: distribution = buildDistribution(scores)
  $: maxCount = Math.max(...(distribution?.bins.map((bin) => bin.count) ?? [1]))
  $: slotWidth = (plotRight - plotLeft) / (distribution?.bins.length ?? 1)
  $: meanX = distribution
    ? plotLeft + (distribution.min === distribution.max ? .5 : (distribution.mean - distribution.min) / (distribution.max - distribution.min)) * (plotRight - plotLeft)
    : plotLeft
</script>

<section class="score-distribution">
  <div class="section-heading">
    <h2>スコア分布</h2>
    <div class="distribution-meta"><strong>{scores.length}件</strong>{#if distribution}<span>{formatScore(distribution.min)} ～ {formatScore(distribution.max)}</span>{/if}</div>
  </div>
  {#if distribution}
    <div class="distribution-chart">
      <svg viewBox={`0 0 ${chartWidth} 220`} role="img" aria-label={`${scores.length}ケースのスコア分布`}>
        <line class="distribution-axis" x1={plotLeft} x2={plotRight} y1={plotBottom} y2={plotBottom}></line>
        {#each distribution.bins as bin, index}
          <rect
            class="distribution-bar"
            x={plotLeft + index * slotWidth + 1}
            y={plotBottom - (bin.count / maxCount) * (plotBottom - plotTop)}
            width={Math.max(1, slotWidth - 2)}
            height={(bin.count / maxCount) * (plotBottom - plotTop)}
          ><title>{formatScore(bin.start)} ～ {formatScore(bin.end)}: {bin.count}件</title></rect>
        {/each}
        <line class="distribution-mean" x1={meanX} x2={meanX} y1={plotTop} y2={plotBottom}></line>
        <text class="distribution-label" x={meanX} y={15} text-anchor="middle">平均 {formatScore(distribution.mean)}</text>
        <text class="distribution-label" x={plotLeft} y={204}>{formatScore(distribution.min)}</text>
        <text class="distribution-label" x={plotRight} y={204} text-anchor="end">{formatScore(distribution.max)}</text>
      </svg>
    </div>
  {:else}
    <div class="empty-state compact"><strong>表示できるスコアがありません</strong></div>
  {/if}
</section>

<style>
  .score-distribution { margin-bottom: 20px; }
  .distribution-meta { display: flex; gap: 10px; color: var(--graphite-soft); font: 11px var(--mono); }
  .distribution-meta span { color: var(--pencil); }
  .distribution-chart { padding: 10px 16px 14px; background: var(--paper); }
  svg { display: block; width: 100%; max-height: 240px; }
  .distribution-axis { stroke: var(--rule-strong); }
  .distribution-bar { fill: var(--pencil); opacity: .68; }
  .distribution-bar:hover { opacity: 1; }
  .distribution-mean { stroke: var(--selection); stroke-width: 2; stroke-dasharray: 5 4; }
  .distribution-label { fill: var(--pencil); font: 10px var(--mono); }
</style>

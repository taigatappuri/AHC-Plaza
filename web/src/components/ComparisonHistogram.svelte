<script lang="ts">
  import { formatScore } from '../lib/formatters'
  import type { Comparison, ComparisonPair } from '../lib/types'

  export let comparison: Comparison

  type Bin = { start: number; end: number; countA: number; countB: number }
  type Histogram = { bins: Bin[]; min: number; max: number }

  const plotLeft = 34
  const plotRight = 980
  const plotTop = 24
  const plotBottom = 178

  function buildHistogram(pairs: ComparisonPair[]): Histogram | null {
    const values = pairs.flatMap((pair) => [pair.a, pair.b]).filter(Number.isFinite)
    if (!values.length) return null
    const actualMin = values.reduce((current, value) => Math.min(current, value))
    const actualMax = values.reduce((current, value) => Math.max(current, value))
    const padding = actualMin === actualMax ? Math.max(Math.abs(actualMin) * .05, 1) : 0
    const min = actualMin - padding
    const max = actualMax + padding
    const binCount = Math.min(64, Math.max(16, Math.ceil(Math.sqrt(pairs.length) * 3)))
    const step = (max - min) / binCount
    const bins = Array.from({ length: binCount }, (_, index) => ({
      start: min + step * index,
      end: min + step * (index + 1),
      countA: 0,
      countB: 0
    }))
    for (const pair of pairs) {
      bins[Math.min(binCount - 1, Math.floor((pair.a - min) / step))].countA += 1
      bins[Math.min(binCount - 1, Math.floor((pair.b - min) / step))].countB += 1
    }
    return { bins, min, max }
  }

  $: histogram = buildHistogram(comparison.pairs)
  $: maxCount = Math.max(...(histogram?.bins.flatMap((bin) => [bin.countA, bin.countB]) ?? [1]))
  $: slotWidth = (plotRight - plotLeft) / (histogram?.bins.length ?? 1)
  const meanX = (value: number) => histogram
    ? plotLeft + ((value - histogram.min) / (histogram.max - histogram.min)) * (plotRight - plotLeft)
    : plotLeft
</script>

<section class="comparison-histogram">
  <header>
    <h4>スコア分布</h4>
    <div class="histogram-legend">
      <span class="run-a"><i>A</i>Run A · 平均 {formatScore(comparison.mean_a)}</span>
      <span class="run-b"><i>B</i>Run B · 平均 {formatScore(comparison.mean_b)}</span>
    </div>
  </header>
  {#if histogram}
    <div class="histogram-chart">
      <svg viewBox="0 0 1000 220" role="img" aria-label="Run AとRun Bのスコア分布">
        <line class="histogram-axis" x1={plotLeft} x2={plotRight} y1={plotBottom} y2={plotBottom}></line>
        {#each histogram.bins as bin, index}
          <rect class="histogram-bar run-a" x={plotLeft + index * slotWidth + 1} y={plotBottom - (bin.countA / maxCount) * (plotBottom - plotTop)} width={Math.max(1, slotWidth - 2)} height={(bin.countA / maxCount) * (plotBottom - plotTop)}><title>Run A · {formatScore(bin.start)} ～ {formatScore(bin.end)}: {bin.countA}件</title></rect>
          <rect class="histogram-bar run-b" x={plotLeft + index * slotWidth + 1} y={plotBottom - (bin.countB / maxCount) * (plotBottom - plotTop)} width={Math.max(1, slotWidth - 2)} height={(bin.countB / maxCount) * (plotBottom - plotTop)}><title>Run B · {formatScore(bin.start)} ～ {formatScore(bin.end)}: {bin.countB}件</title></rect>
        {/each}
        <line class="histogram-mean run-a" x1={meanX(comparison.mean_a)} x2={meanX(comparison.mean_a)} y1={plotTop} y2={plotBottom}></line>
        <line class="histogram-mean run-b" x1={meanX(comparison.mean_b)} x2={meanX(comparison.mean_b)} y1={plotTop} y2={plotBottom}></line>
        <text class="histogram-label" x={plotLeft} y="204">{formatScore(histogram.min)}</text>
        <text class="histogram-label" x={plotRight} y="204" text-anchor="end">{formatScore(histogram.max)}</text>
      </svg>
    </div>
  {/if}
</section>

<style>
  .comparison-histogram { border-top: 1px solid var(--rule); }
  header { display: flex; align-items: center; justify-content: space-between; gap: 20px; padding: 16px 20px 0; }
  h4 { margin: 0; font-size: 14px; }
  .histogram-legend { display: flex; gap: 16px; font: 600 11px var(--mono); }
  .histogram-legend span { display: flex; align-items: center; gap: 6px; }
  .histogram-legend i { display: inline-grid; place-items: center; width: 18px; height: 18px; color: #fff; border-radius: 3px; font-style: normal; }
  .histogram-legend .run-a { color: var(--run-a); }
  .histogram-legend .run-b { color: var(--run-b); }
  .histogram-legend .run-a i { background: var(--run-a); }
  .histogram-legend .run-b i { background: var(--run-b); }
  .histogram-chart { padding: 4px 18px 12px; }
  svg { display: block; width: 100%; max-height: 240px; }
  .histogram-axis { stroke: var(--rule-strong); }
  .histogram-bar { stroke-width: 1; opacity: .46; }
  .histogram-bar:hover { opacity: .75; }
  .histogram-bar.run-a { fill: var(--run-a); stroke: var(--run-a); }
  .histogram-bar.run-b { fill: var(--run-b); stroke: var(--run-b); }
  .histogram-mean { stroke-width: 2; stroke-dasharray: 5 4; }
  .histogram-mean.run-a { stroke: var(--run-a); }
  .histogram-mean.run-b { stroke: var(--run-b); }
  .histogram-label { fill: var(--pencil); font: 10px var(--mono); }
</style>

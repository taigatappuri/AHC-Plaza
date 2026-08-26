import assert from 'node:assert/strict'
import test from 'node:test'

import { filterCaseResultsByFeatures, scoreDistributionCaseResults } from '../src/lib/feature-filter.js'

const caseResults = [
  { input_case_id: '0000', score: 100 },
  { input_case_id: '0001', score: 200 },
  { input_case_id: '0002', score: 300 }
]

const featureCases = [
  { input_case_id: '0000', values: { wall_count: 4, size: 10 } },
  { input_case_id: '0001', values: { wall_count: 8, size: 20 } },
  { input_case_id: '0002', values: { wall_count: 12, size: 20 } }
]

test('入力特徴量で絞り込んだケースだけをスコア分布へ渡す', () => {
  const filtered = filterCaseResultsByFeatures(caseResults, featureCases, [
    { feature: 'wall_count', operator: '<=', value: 8 }
  ])

  assert.deepEqual(scoreDistributionCaseResults(caseResults, filtered), caseResults.slice(0, 2))
})

test('複数の入力特徴量条件をANDで適用する', () => {
  const filtered = filterCaseResultsByFeatures(caseResults, featureCases, [
    { feature: 'wall_count', operator: '>', value: 4 },
    { feature: 'size', operator: '=', value: 20 },
    { feature: 'wall_count', operator: '!=', value: 12 }
  ])

  assert.deepEqual(filtered, [caseResults[1]])
})

test('有効な絞り込みがない場合は全ケースのスコア分布へ戻す', () => {
  assert.strictEqual(scoreDistributionCaseResults(caseResults, null), caseResults)
})

test('一致するケースがない場合は空のスコア分布にする', () => {
  assert.deepEqual(scoreDistributionCaseResults(caseResults, []), [])
})

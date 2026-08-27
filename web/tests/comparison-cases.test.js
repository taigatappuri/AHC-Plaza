import assert from 'node:assert/strict'
import test from 'node:test'

import { comparisonCasesForResult } from '../src/lib/comparison-cases.js'

const cases = [{ id: '0000' }, { id: '0001' }, { id: '0002' }]

const comparison = (active, inputCaseIDs) => ({
  case_count: inputCaseIDs.length,
  mean_a: 0,
  mean_b: 0,
  mean_improvement: 0,
  improvement_rate: 0,
  statistical_method: 'paired_bootstrap_t',
  inference_available: true,
  p_value: 1,
  confidence_level: 0.95,
  confidence_low: 0,
  confidence_high: 0,
  significant: false,
  filter: {
    active,
    conditions: [],
    original_case_count: cases.length,
    matched_case_count: inputCaseIDs.length
  },
  pairs: inputCaseIDs.map((input_case_id) => ({ input_case_id, a: 0, b: 0, difference: 0 }))
})

test('入力条件が有効なときは比較対象になったケースだけを返す', () => {
  assert.deepEqual(comparisonCasesForResult(cases, comparison(true, ['0002', '0000'])), [cases[0], cases[2]])
})

test('入力条件がないときはすべての共通ケースを返す', () => {
  assert.deepEqual(comparisonCasesForResult(cases, comparison(false, ['0000', '0001', '0002'])), cases)
})

test('比較結果がないときはすべての共通ケースを返す', () => {
  assert.deepEqual(comparisonCasesForResult(cases, null), cases)
})

import test from 'node:test'
import assert from 'node:assert/strict'
import { mergeTrialHistory, trialHistoryLimit, trialHistoryOffset } from '../src/lib/tuning-history.js'

test('1万件の履歴更新は末尾の状態と追加分だけを取得して結合する', () => {
  const history = Array.from({ length: 9999 }, (_, number) => ({ number, status: 'COMPLETE' }))
  const offset = trialHistoryOffset(history.length, 10000)
  assert.equal(offset, 9998)
  const merged = mergeTrialHistory(history, [{ number: 9998, status: 'FAIL' }, { number: 9999, status: 'RUNNING' }], offset)
  assert.equal(merged.length, 10000)
  assert.equal(merged[9998].status, 'FAIL')
  assert.equal(merged[9999].status, 'RUNNING')
})

test('Study切替などで履歴件数が戻った場合は先頭から取得する', () => {
  assert.equal(trialHistoryOffset(100, 3), 0)
})

test('terminal 99件とRUNNING 1件がある場合もterminal件数を越えて取得しない', () => {
  const offset = trialHistoryOffset(99, 99)
  assert.equal(offset, 98)
  assert.equal(trialHistoryLimit(offset, 99), 1)
})

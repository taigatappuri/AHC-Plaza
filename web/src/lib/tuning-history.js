/**
 * @template T
 * @param {T[]} history
 * @param {T[]} incoming
 * @param {number} offset
 * @returns {T[]}
 */
export function mergeTrialHistory(history, incoming, offset) {
  return [...history.slice(0, offset), ...incoming]
}

/** @param {number} historyLength @param {number} total */
export function trialHistoryOffset(historyLength, total) {
  if (historyLength > total) return 0
  return historyLength > 0 ? historyLength - 1 : 0
}

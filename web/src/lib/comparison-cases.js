/**
 * @template {{ id: string }} T
 * @param {T[]} cases
 * @param {import('./types').Comparison | null} comparison
 * @returns {T[]}
 */
export const comparisonCasesForResult = (cases, comparison) => {
  if (!comparison?.filter.active) return cases

  const matchedCaseIDs = new Set(comparison.pairs.map((pair) => pair.input_case_id))
  return cases.filter((item) => matchedCaseIDs.has(item.id))
}

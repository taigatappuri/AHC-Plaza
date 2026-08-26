/**
 * @param {number} value
 * @param {import('./types').FeatureCondition} condition
 */
const matchesCondition = (value, condition) => ({
  '<': value < condition.value,
  '<=': value <= condition.value,
  '=': value === condition.value,
  '!=': value !== condition.value,
  '>=': value >= condition.value,
  '>': value > condition.value
})[condition.operator]

/**
 * @param {import('./types').CaseResult[]} caseResults
 * @param {import('./types').FeatureCaseValues[]} featureCases
 * @param {import('./types').FeatureCondition[]} conditions
 * @returns {import('./types').CaseResult[]}
 */
export const filterCaseResultsByFeatures = (caseResults, featureCases, conditions) => {
  const valuesByCase = new Map(featureCases.map((item) => [item.input_case_id, item.values]))
  return caseResults.filter((result) => conditions.every((condition) => {
    const value = valuesByCase.get(result.input_case_id)?.[condition.feature]
    return typeof value === 'number' && Number.isFinite(value) && matchesCondition(value, condition)
  }))
}

/**
 * @param {import('./types').CaseResult[]} caseResults
 * @param {import('./types').CaseResult[] | null} filteredCaseResults
 * @returns {import('./types').CaseResult[]}
 */
export const scoreDistributionCaseResults = (caseResults, filteredCaseResults) =>
  filteredCaseResults ?? caseResults

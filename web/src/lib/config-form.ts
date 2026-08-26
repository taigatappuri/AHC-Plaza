import type { ConfigData } from './types'

export const cloneConfig = (value: ConfigData): ConfigData => {
  const cloned = JSON.parse(JSON.stringify(value)) as ConfigData
  cloned.input_format.features ??= []
  return cloned
}

const validateRelativePath = (value: string, label: string) => {
  const trimmed = value.trim()
  if (!trimmed) return `${label}を入力`
  if (trimmed.startsWith('/')) return '相対パスを入力'
  let depth = 0
  for (const part of trimmed.split('/')) {
    if (!part || part === '.') continue
    if (part === '..') {
      depth -= 1
      if (depth < 0) return 'プロジェクト外は指定不可'
    } else {
      depth += 1
    }
  }
  return ''
}

export function validateConfig(value: ConfigData) {
  const errors: Record<string, string> = {}
  if (!value.project.problem.trim()) errors['project.problem'] = '問題名を入力'
  if (!['max', 'min'].includes(value.project.objective)) errors['project.objective'] = '最大化または最小化を選択'

  const paths: [string, string, string][] = [
    ['paths.solver_dir', value.paths.solver_dir, 'solverディレクトリ'],
    ['paths.tools_dir', value.paths.tools_dir, 'toolsディレクトリ'],
    ['execution.default_input_dir', value.execution.default_input_dir, '入力セットのディレクトリ'],
    ['pahcer.setting_file', value.pahcer.setting_file, 'pahcer設定ファイル']
  ]
  for (const [key, path, label] of paths) {
    const error = validateRelativePath(path, label)
    if (error) errors[key] = error
  }

  if (!Number.isInteger(value.execution.threads) || value.execution.threads < 0) {
    errors['execution.threads'] = '0以上の整数を入力'
  }
  if (!Number.isInteger(value.execution.timeout_ms) || value.execution.timeout_ms < 1) {
    errors['execution.timeout_ms'] = '1以上の整数を入力'
  }
  if (!Number.isFinite(value.score.invalid_score)) errors['score.invalid_score'] = '数値を入力'
  if (!(value.statistics.confidence_level > 0 && value.statistics.confidence_level < 1)) {
    errors['statistics.confidence_level'] = '0〜1の値を入力'
  }
  if (!Number.isInteger(value.statistics.bootstrap_iterations) || value.statistics.bootstrap_iterations < 1) {
    errors['statistics.bootstrap_iterations'] = '1以上の整数を入力'
  }

  const names = new Map<string, string>()
  const validateName = (name: string, prefix: string, requiredLabel: string, duplicateLabel: string) => {
    const trimmed = name.trim()
    if (!trimmed) errors[`${prefix}.name`] = `${requiredLabel}名を入力`
    else if (trimmed.length > 64 || /[\r\n\t]/.test(trimmed)) errors[`${prefix}.name`] = '64文字以下の1行で入力'
    else if (names.has(trimmed)) errors[`${prefix}.name`] = `${names.get(trimmed)}と重複`
    else names.set(trimmed, duplicateLabel)
  }
  const positions = new Map<string, number>()
  value.input_format.variables.forEach((variable, index) => {
    const prefix = `input_format.variables.${index}`
    validateName(variable.name, prefix, '変数', `位置変数${index + 1}`)
    if (!Number.isInteger(variable.line) || variable.line < 1) {
      errors[`${prefix}.line`] = '1以上の整数を入力'
    }
    if (!Number.isInteger(variable.column) || variable.column < 1) {
      errors[`${prefix}.column`] = '1以上の整数を入力'
    }
    if (Number.isInteger(variable.line) && variable.line > 0 && Number.isInteger(variable.column) && variable.column > 0) {
      const position = `${variable.line}:${variable.column}`
      if (positions.has(position)) {
        errors[`${prefix}.position`] = `${positions.get(position)! + 1}番と同じ位置`
      } else {
        positions.set(position, index)
      }
    }
  })

  const featureSources = new Map<string, number>()
  value.input_format.features.forEach((feature, index) => {
    const prefix = `input_format.features.${index}`
    validateName(feature.name, prefix, '特徴量', `C++特徴量${index + 1}`)
    if (!feature.source.startsWith('features/') || !feature.source.toLowerCase().endsWith('.cpp')) {
      errors[`${prefix}.source`] = 'features配下の.cppを選択'
    } else if (featureSources.has(feature.source)) {
      errors[`${prefix}.source`] = `${featureSources.get(feature.source)! + 1}番と重複`
    } else {
      featureSources.set(feature.source, index)
    }
    if (!Number.isInteger(feature.timeout_ms) || feature.timeout_ms < 1 || feature.timeout_ms > 60000) {
      errors[`${prefix}.timeout_ms`] = '1〜60000msの整数を入力'
    }
  })
  return errors
}

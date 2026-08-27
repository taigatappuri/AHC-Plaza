export type Tab = 'overview' | 'input' | 'detail' | 'compare' | 'settings'

export type InputGenerator = {
  name: string
  dir: string
  source: string
}

export type InputParameter = {
  name: string
  constraint: string
}

export type InputGenerateRequest = {
  generator_dir: string
  generator: string
  case_count: number
  seed_start: number
  input_directory: string
  overwrite: boolean
  parameters: InputParameter[]
}

export type InputGenerateResult = {
  output_dir: string
  case_count: number
}

export type RunStatistics = {
  case_count: number
  average_score: number
  min_score: number
  q1_score: number
  median_score: number
  q3_score: number
  max_score: number
  variance_score: number
  stddev_score: number
  average_execution_time_ns: number
  max_execution_time_ns: number
}

export type Run = {
  id: string
  run_number: number
  solver_path: string
  input_dir: string
  status: string
  comment: string
  created_at: string
  source_hash?: string
} & Partial<RunStatistics>

export type CaseResult = {
  input_case_id: string
  seed: number
  score: number
  execution_time_ns: number
  status: string
}

export type ConfigData = {
  project: {
    problem: string
    objective: 'max' | 'min'
  }
  paths: {
    solver_dir: string
    tools_dir: string
  }
  execution: {
    default_input_dir: string
    threads: number
    timeout_ms: number
  }
  pahcer: {
    setting_file: string
  }
  score: {
    invalid_score: number
    include_invalid_cases: boolean
  }
  statistics: {
    confidence_level: number
    bootstrap_iterations: number
  }
  input_format: {
    variables: InputVariable[]
    features: InputFeatureConfig[]
  }
}

export type InputVariable = {
  name: string
  line: number
  column: number
}

export type InputFeatureConfig = {
  name: string
  source: string
  timeout_ms: number
}

export type InputFeatureDefinition = {
  name: string
  kind: 'position' | 'cpp'
  line?: number
  column?: number
  source?: string
}

export type FeatureCaseValues = {
  input_case_id: string
  values: Record<string, number>
}

export type FeatureData = {
  features: InputFeatureDefinition[]
  cases: FeatureCaseValues[]
}

export type FeatureConditionOperator = '<' | '<=' | '=' | '!=' | '>=' | '>'

export type FeatureCondition = {
  feature: string
  operator: FeatureConditionOperator
  value: number
}

export type ComparisonFilter = {
  active: boolean
  conditions: FeatureCondition[]
  original_case_count: number
  matched_case_count: number
}

export type ComparisonPair = {
  input_case_id: string
  a: number
  b: number
  difference: number
}

export type Comparison = {
  case_count: number
  mean_a: number
  mean_b: number
  mean_improvement: number
  improvement_rate: number
  statistical_method: 'paired_bootstrap_t'
  inference_available: boolean
  inference_note?: string
  p_value: number | null
  confidence_level: number
  confidence_low: number | null
  confidence_high: number | null
  significant: boolean
  filter: ComparisonFilter
  pairs: ComparisonPair[]
}

export type TabDefinition = {
  id: Tab
  label: string
  hint: string
}

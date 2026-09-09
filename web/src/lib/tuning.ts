export type TuneParameter = {
  name: string; type: string; current: number; low: number | null; high: number | null;
  step: number | null; log: boolean; enabled: boolean; line: number; start: number; end: number; token: string;
}
export type TuneScan = { hash: string; source: string; parameters: TuneParameter[] }
export type TuneStudy = {
  id: string; status: string; error: string; trials: number; completed: number; failed: number;
  seconds: number; elapsed: number; baseline_run: string; baseline_value: number | null;
  best_run: string; best_value: number | null; best_params: Record<string, number>;
  validations: { baseline_run: string; candidate_run: string; input_dir: string; error: string }[];
}
export type TuneTrial = { number: number; run_id: string; status: string; params: Record<string, number>; value: number | null; reason: string }
export type TuneEnvironment = { available: boolean; ready: boolean; path: string; python: string; optuna: string; compressed_bytes: number; unpacked_bytes: number }
export type TuneDetail = { worker_log: string; study: TuneStudy; active: boolean; usage_bytes: number; manifest: { request: { solver: string; input_dir: string; threads: number; timeout_ms: number }; parameters: TuneParameter[]; prepared: { config: { File: { project: { objective: string } } }; inputs: unknown[] } } }

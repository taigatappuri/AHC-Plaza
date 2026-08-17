<script lang="ts">
  import type { InputGenerateRequest, InputGenerateResult, InputGenerator, InputParameter } from '../lib/types'

  export let generators: InputGenerator[] = []
  export let inputGeneratorLoading = false
  export let error = ''
  export let result: InputGenerateResult | null = null
  export let defaultOutputDir = 'ahc-plaza/inputs'
  export let loading = false
  export let onGenerate: (request: InputGenerateRequest) => void = () => {}

  let generator = ''
  let generatorDir = 'tools'
  let caseCount = 100
  let seedStart = 0
  let inputDirectoryName = ''
  let overwrite = false
  let parameters: InputParameter[] = []
  let attempted = false

  $: if (generators[0] && !generators.some((item) => item.name === generator)) selectGenerator(generators[0])
  $: localError = validate(generator, caseCount, seedStart, inputDirectoryName, parameters)

  function selectGenerator(option: InputGenerator) {
    generator = option.name
    generatorDir = option.dir
  }

  function validDirectory(value: string) {
    const name = value.trim()
    return name.length > 0 && name !== '.' && name !== '..' && !name.startsWith('.') && [...name].length <= 64 && !/[\/\\\u0000-\u001f:*?"<>|]/.test(name)
  }

  function validate(selectedGenerator: string, count: number, firstSeed: number, directory: string, values: InputParameter[]) {
    if (!selectedGenerator) return '使用できる生成器がありません'
    if (!Number.isInteger(count) || count < 1 || count > 10000) return 'ケース数は1〜10,000'
    if (!Number.isInteger(firstSeed) || firstSeed < 0) return '開始seedは0以上の整数'
    if (!validDirectory(directory)) return '出力先の名前を確認してください'
    if (values.some((parameter) => !parameter.name.trim() || !parameter.constraint.trim())) return 'パラメータ名と条件式を入力してください'
    return ''
  }

  function submit() {
    attempted = true
    if (localError || loading) return
    onGenerate({
      generator_dir: generatorDir,
      generator,
      case_count: caseCount,
      seed_start: seedStart,
      input_directory: inputDirectoryName.trim(),
      overwrite,
      parameters: parameters.map((parameter) => ({ name: parameter.name.trim(), constraint: parameter.constraint.trim() }))
    })
  }
</script>

{#if error}<div class="input-error" role="alert">{error}</div>{/if}

<form class="input-recipe" onsubmit={(event) => { event.preventDefault(); submit() }}>
  <div class="input-fields">
    <label class="input-field input-field-wide"><span class="field-label">入力生成器</span><select bind:value={generator} disabled={inputGeneratorLoading || !generators.length} onchange={() => { const option = generators.find((item) => item.name === generator); if (option) selectGenerator(option) }}><option value="">使用できる生成器がありません</option>{#each generators as option}<option value={option.name}>{option.name} · {option.source}</option>{/each}</select></label>
    <label class="input-field"><span class="field-label">ケース数</span><input type="number" min="1" max="10000" bind:value={caseCount} /></label>
    <label class="input-field"><span class="field-label">開始seed</span><input type="number" min="0" bind:value={seedStart} /></label>
    <label class="input-field input-field-wide"><span class="field-label">出力先</span><div class="path-input"><span>{defaultOutputDir}/</span><input bind:value={inputDirectoryName} placeholder="baseline" /></div></label>

    <div class="input-constraints">
      <div class="input-constraints-heading"><span class="field-label">パラメータ条件</span><button type="button" class="text-action" onclick={() => parameters = [...parameters, { name: '', constraint: '' }]}>条件を追加</button></div>
      {#each parameters as parameter, index}
        <div class="input-constraint-row"><span class="input-constraint-index">{index + 1}</span><input bind:value={parameter.name} placeholder="名前" /><input bind:value={parameter.constraint} placeholder="N >= 50" /><button type="button" class="remove-action" onclick={() => parameters = parameters.filter((_, itemIndex) => itemIndex !== index)}>削除</button></div>
      {/each}
    </div>
    <label class="input-switch-row"><input type="checkbox" bind:checked={overwrite} />既存ファイルを上書き</label>
  </div>

  {#if attempted && localError}<p class="input-local-error">{localError}</p>{/if}
  {#if result}<div class="input-result">{result.output_dir} に {result.case_count}件生成しました</div>{/if}
  <div class="input-submit-row"><button class="primary-action" disabled={loading || inputGeneratorLoading}>{loading ? '生成中…' : '入力を生成'}</button></div>
</form>

<style>
  .input-error, .input-recipe { width: 100%; }
</style>

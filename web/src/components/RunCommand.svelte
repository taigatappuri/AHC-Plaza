<script lang="ts">
  export let solver = ''
  export let solvers: string[] = []
  export let solverLoading = false
  export let solverError = ''
  export let onRefreshSolvers: () => void = () => {}
  export let inputDirectories: string[] = []
  export let inputRoot = 'ahc-plaza/inputs'
  export let inputDirectoryLoading = false
  export let inputDirectoryError = ''
  export let onRefreshInputDirectories: () => void = () => {}
  export let inputDir = ''
  export let threads = 0
  export let timeoutMilliseconds = 300000
  export let comment = ''
  export let loading = false
  export let defaultsAvailable = false
  export let onResetDefaults: () => void = () => {}
  export let onSubmit: () => void = () => {}
</script>

<form class="run-launcher" onsubmit={(event) => { event.preventDefault(); onSubmit() }}>
  <div class="command-form">
    <label>
      <span class="field-label">ソースファイル</span>
      <div class="command-picker">
        <select bind:value={solver} disabled={solverLoading || solvers.length === 0} aria-label="ソースファイル">
          <option value="" disabled>{solverLoading ? '読み込み中…' : 'ソースファイルを選択'}</option>
          {#each solvers as solverPath}
            <option value={solverPath}>{solverPath}</option>
          {/each}
        </select>
        <button type="button" class="picker-refresh" aria-label="ソースファイル一覧を再読込" onclick={onRefreshSolvers} disabled={solverLoading}>再読込</button>
      </div>
      {#if solverError}<small class="command-field-error">{solverError}</small>{/if}

    </label>
    <label>
      <span class="field-label">入力セット</span>
      <div class="command-picker">
        <select bind:value={inputDir} disabled={inputDirectoryLoading || inputDirectories.length === 0} aria-label="入力ディレクトリ">
          <option value="" disabled>{inputDirectoryLoading ? '読み込み中…' : inputDirectories.length ? '入力セットを選択' : `入力セットなし: ${inputRoot}`}</option>
          {#each inputDirectories as directory}
            <option value={directory}>{directory}</option>
          {/each}
        </select>
        <button type="button" class="picker-refresh" aria-label="入力一覧を再読込" onclick={onRefreshInputDirectories} disabled={inputDirectoryLoading}>再読込</button>
      </div>
      {#if inputDirectoryError}<small class="command-field-error">{inputDirectoryError}</small>{/if}
    </label>
    <div class="execution-options">
      <div class="execution-options-header">
        <span class="field-label">実行設定</span>
        <button class="text-action defaults-action" type="button" onclick={onResetDefaults} disabled={!defaultsAvailable} title="ahc-plaza.tomlの実行設定に戻す">デフォルトに戻す</button>
      </div>
      <div class="execution-option-fields">
        <label><span class="option-label">各ケースのタイムアウト（ミリ秒）</span><input type="number" min="1" bind:value={timeoutMilliseconds} /></label>
        <label><span class="option-label">スレッド数</span><input type="number" min="0" bind:value={threads} title="0で自動設定" /></label>
      </div>
    </div>
    <label class="comment-field"><span class="field-label">コメント</span><input bind:value={comment} placeholder="コメント" /></label>
    <button class="primary-action launch-action" type="submit" disabled={loading || solverLoading || !solver || !inputDir}>{loading ? '実行中…' : '実行'}</button>
  </div>
</form>

<style>
  .run-launcher { margin-bottom: 36px; }
  .command-form { display: grid; grid-template-columns: minmax(210px, 1.2fr) minmax(210px, 1.2fr) minmax(280px, .95fr) 132px; align-items: end; gap: 12px; }
  .command-form label { display: grid; gap: 7px; min-width: 0; }
  .command-picker { display: flex; }
  .command-picker select { min-width: 0; }
  .picker-refresh { flex: 0 0 58px; margin-left: 6px; color: var(--graphite-soft); background: transparent; border: 1px solid var(--rule-strong); border-radius: 4px; font-size: 11px; }
  .picker-refresh:hover { color: var(--selection); border-color: var(--selection); }
  .execution-options { display: grid; gap: 7px; min-width: 0; }
  .execution-options-header { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 18px; }
  .execution-option-fields { display: grid; grid-template-columns: minmax(180px, 1fr) 88px; gap: 8px; }
  .option-label { color: var(--pencil); font-size: 10px; white-space: nowrap; }
  .defaults-action { flex: 0 0 auto; }
  .comment-field { grid-column: 1 / 4; }
  .launch-action { grid-column: 4; grid-row: 1; min-height: 38px; }
  .command-field-error { color: var(--failure); font-size: 11px; }

  @media (max-width: 1150px) {
    .command-form { grid-template-columns: repeat(2, minmax(200px, 1fr)) 132px; }
    .execution-options { grid-column: 1 / 3; }
    .execution-option-fields { grid-template-columns: minmax(220px, 1fr) minmax(140px, .65fr); }
    .comment-field { grid-column: 1 / 3; }
    .launch-action { grid-column: 3; grid-row: 1; }
  }
</style>

<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { errorMessage, requestJSON, requestText } from '../lib/api'
  import type { CaseResult, Run } from '../lib/types'

  export let selectedRun: Run | null = null
  export let caseResults: CaseResult[] = []
  export let selectedCaseID = ''

  let visualizerReady = false
  let entry = ''
  let visualizerURL = ''
  let selectedCase: CaseResult | null = null
  let selectedSeed = 0
  let iframeSource = 'about:blank'
  let selectionKey = ''
  let input = ''
  let output = ''
  let downloading = false
  let loadingCase = false
  let deleting = false
  let scale = 100
  let error = ''
  let notice = ''
  let cacheKey = Date.now()
  let iframe: HTMLIFrameElement | undefined
  let iframeLoaded = false
  let loadingKey = ''
  let pendingApply = false
  let requestSerial = 0
  const emptyCase: CaseResult = {
    input_case_id: '',
    seed: 0,
    score: 0,
    execution_time_ns: 0,
    status: 'pending'
  }
  const artifact = (kind: 'case-input' | 'case-output', runID: string, caseID: string, allowMissing = false) =>
    requestText(`/api/runs/${encodeURIComponent(runID)}/${kind}?case_id=${encodeURIComponent(caseID)}`, allowMissing)

  $: if (caseResults.length > 0 && !caseResults.some((item) => item.input_case_id === selectedCaseID)) {
    selectedCaseID = caseResults[0].input_case_id
  }
  $: if (caseResults.length === 0) selectedCaseID = ''
  $: selectedCase = caseResults.length > 0
    ? caseResults.find((item) => item.input_case_id === selectedCaseID) ?? null
    : emptyCase
  $: selectedSeed = selectedCase?.seed ?? 0
  $: iframeSource = visualizerReady && entry ? `/visualizer/${encodeURIComponent(entry)}?v=${cacheKey}` : 'about:blank'
  $: selectionKey = visualizerReady && selectedCase ? `${selectedRun?.id ?? 'empty'}:${selectedCase.input_case_id || 'empty'}:${entry}` : ''
  $: if (selectionKey && selectionKey !== loadingKey) {
    loadingKey = selectionKey
    void loadSelectedCase(selectionKey)
  }

  onMount(() => {
    void refreshStatus()
  })

  async function refreshStatus() {
    try {
      const status = await requestJSON<{ ready: boolean; entry?: string }>('/api/visualizer')
      visualizerReady = status.ready
      entry = status.entry ?? ''
      cacheKey = Date.now()
      error = ''
    } catch (reason) {
      error = errorMessage(reason)
    }
  }

  async function downloadVisualizer() {
    if (!/^https:\/\/img\.atcoder\.jp\/.*\.html(?:\?.*)?$/.test(visualizerURL)) {
      error = 'img.atcoder.jp のHTML URLを入力'
      return
    }
    downloading = true
    error = ''
    notice = ''
    try {
      const result = await requestJSON<{ entry: string; files: string[] }>('/api/visualizer/download', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url: visualizerURL })
      })
      entry = result.entry
      visualizerReady = true
      iframeLoaded = false
      cacheKey = Date.now()
      notice = `ビジュアライザを取得: ${result.files.length}件`
    } catch (reason) {
      error = errorMessage(reason)
    } finally {
      downloading = false
    }
  }

  async function deleteVisualizer() {
    deleting = true
    error = ''
    try {
      await requestJSON('/api/visualizer', { method: 'DELETE' })
      visualizerReady = false
      entry = ''
      iframeLoaded = false
      notice = 'ビジュアライザを削除'
    } catch (reason) {
      error = errorMessage(reason)
    } finally {
      deleting = false
    }
  }

  async function loadSelectedCase(key: string) {
    if (!selectedCase) return
    const serial = ++requestSerial
    const caseID = selectedCase.input_case_id
    loadingCase = true
    error = ''
    try {
      const [nextInput, nextOutput] = selectedRun && caseID
        ? await Promise.all([
          artifact('case-input', selectedRun.id, caseID),
          artifact('case-output', selectedRun.id, caseID, true)
        ])
        : ['', '']
      if (serial !== requestSerial || key !== selectionKey) return
      input = nextInput
      output = nextOutput
      pendingApply = true
      await applyToIframe(key)
    } catch (reason) {
      if (serial === requestSerial) error = errorMessage(reason)
    } finally {
      if (serial === requestSerial) loadingCase = false
    }
  }

  async function handleIframeLoad() {
    iframeLoaded = true
    const document = iframe?.contentDocument ?? iframe?.contentWindow?.document
    if (document) applyVisualizerTheme(document)
    await applyToIframe()
  }

  function applyVisualizerTheme(document: Document) {
    let fontFaces = ''
    for (const sheet of Array.from(window.document.styleSheets)) {
      try {
        for (const rule of Array.from(sheet.cssRules)) {
          if (rule instanceof CSSFontFaceRule) fontFaces += rule.cssText
        }
      } catch {
        // 外部スタイルシートは参照できないため無視します。
      }
    }
    const googleFontLink = document.getElementById('ahc-plaza-visualizer-google-font') as HTMLLinkElement | null
    const link = googleFontLink ?? document.createElement('link')
    link.id = 'ahc-plaza-visualizer-google-font'
    link.rel = 'stylesheet'
    link.href = 'https://fonts.googleapis.com/css2?family=Noto+Sans+JP:wght@400;500;600&display=swap'
    if (!link.isConnected) document.head.appendChild(link)
    const style = document.getElementById('ahc-plaza-visualizer-theme') as HTMLStyleElement | null ?? document.createElement('style')
    style.id = 'ahc-plaza-visualizer-theme'
    style.textContent = `${fontFaces}
      html, body, button, input, select { font-family: "Noto Sans JP", sans-serif !important; }
      pre, code, textarea { font-family: "IBM Plex Mono", monospace !important; }
      body { color: #1c2024; }
      button, input, select, textarea { border-color: #c9cfd4; border-radius: 3px; }
      button { color: #3f474d; background: #eceff0; }
      a { color: #1265b0; }
    `
    if (!style.isConnected) document.head.appendChild(style)
  }

  async function waitForVisualizerReady(document: Document) {
    const result = document.getElementById('result')
    if (!result) {
      await new Promise((resolve) => window.setTimeout(resolve, 250))
      return
    }
    for (let attempt = 0; attempt < 40 && result.innerHTML.trim() === ''; attempt += 1) {
      await new Promise((resolve) => window.setTimeout(resolve, 50))
    }
  }

  async function applyToIframe(expectedKey = selectionKey) {
    if (!iframeLoaded || !iframe || !selectedCase || !pendingApply) return
    const serial = requestSerial
    const caseToApply = selectedCase
    const inputToApply = input
    const outputToApply = output
    for (let attempt = 0; attempt < 40; attempt += 1) {
      if (serial !== requestSerial || expectedKey !== selectionKey || caseToApply.input_case_id !== selectedCaseID) return
      const iframeDocument = iframe.contentDocument ?? iframe.contentWindow?.document
      const inputField = iframeDocument?.getElementById('input') as HTMLTextAreaElement | null
      const outputField = iframeDocument?.getElementById('output') as HTMLTextAreaElement | null
      const seedField = iframeDocument?.getElementById('seed') as HTMLInputElement | null
      if (iframeDocument && inputField && outputField && seedField) {
        applyVisualizerTheme(iframeDocument)
        await waitForVisualizerReady(iframeDocument)
        if (serial !== requestSerial || expectedKey !== selectionKey || caseToApply.input_case_id !== selectedCaseID) return
        await tick()
        // seedの変更イベントは入力を再生成してしまうため、表示値だけを更新します。
        seedField.value = String(caseToApply.seed)
        inputField.value = inputToApply
        inputField.dispatchEvent(new Event('input', { bubbles: true }))
        inputField.dispatchEvent(new Event('change', { bubbles: true }))
        outputField.value = outputToApply
        outputField.dispatchEvent(new Event('input', { bubbles: true }))
        outputField.dispatchEvent(new Event('change', { bubbles: true }))
        pendingApply = false
        return
      }
      await new Promise((resolve) => window.setTimeout(resolve, 50))
    }
    error = 'ビジュアライザの入力欄を取得できません'
  }

  function handleCaseChange(event: Event) {
    selectedCaseID = (event.currentTarget as HTMLSelectElement).value
    pendingApply = false
  }
</script>

<section class="panel visualizer-panel">
  <div class="panel-header visualizer-header">
    <h2>ビジュアライザ</h2>
    {#if visualizerReady}
      <div class="visualizer-actions">
        <span class="visualizer-entry">{entry}</span>
        <button class="text-action" type="button" onclick={deleteVisualizer} disabled={deleting}>{deleting ? '削除中…' : '削除'}</button>
      </div>
    {/if}
  </div>

  {#if error}<div class="visualizer-error">{error}</div>{/if}
  {#if notice}<div class="visualizer-notice">{notice}</div>{/if}

  {#if !visualizerReady}
    <div class="visualizer-setup">
      <div>
        <strong>公式ビジュアライザ</strong>
      </div>
      <div class="visualizer-download-form">
        <input bind:value={visualizerURL} placeholder="https://img.atcoder.jp/ahc000/visualizer.html" aria-label="ビジュアライザURL" />
        <button class="primary-action" type="button" onclick={downloadVisualizer} disabled={downloading}>{downloading ? '取得中…' : '取得'}</button>
      </div>
    </div>
  {:else}
    <div class="visualizer-toolbar">
      <label>
        <span class="field-label">ケース</span>
        <select value={selectedCaseID} onchange={handleCaseChange} disabled={caseResults.length === 0}>
          {#if caseResults.length === 0}
            <option value="">ケースなし · seed 0</option>
          {:else}
            {#each caseResults as item}
              <option value={item.input_case_id}>{item.input_case_id} · seed {item.seed}</option>
            {/each}
          {/if}
        </select>
      </label>
      <div class="visualizer-meta"><span>seed</span><strong>{selectedSeed}</strong></div>
      <label class="visualizer-scale">
        <span class="field-label">倍率</span>
        <input type="range" min="50" max="150" step="5" bind:value={scale} />
        <strong>{scale}%</strong>
      </label>
      {#if loadingCase}<span class="visualizer-loading">入出力を読み込み中…</span>{/if}
    </div>
    <div class="visualizer-frame-wrap" class:is-zoomed-out={Number(scale) < 100}>
      <div class="visualizer-frame-scale" style={`transform: scale(${Number(scale) / 100}); width: ${10000 / Number(scale)}%; height: ${10000 / Number(scale)}%;`}>
        <iframe bind:this={iframe} src={iframeSource} title="AHCビジュアライザ" onload={handleIframeLoad} sandbox="allow-scripts allow-same-origin"></iframe>
      </div>
    </div>
  {/if}
</section>

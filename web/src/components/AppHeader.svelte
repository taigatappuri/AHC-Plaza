<script lang="ts">
  import type { Tab, TabDefinition } from '../lib/types'

  export let tabs: TabDefinition[] = []
  export let activeTab: Tab
  export let problem = ''
  export let onNavigate: (tab: Tab) => void = () => {}
</script>

<header class="app-header">
  <div class="product-context">
    <strong>AHC Plaza</strong>
    <span>{problem || 'project'}</span>
  </div>

  <nav aria-label="メインナビゲーション">
    {#each tabs as tab}
      <button
        class:active={activeTab === tab.id}
        title={tab.hint}
        aria-current={activeTab === tab.id ? 'page' : undefined}
        onclick={() => onNavigate(tab.id)}
      >{tab.label}</button>
    {/each}
  </nav>
</header>

<style>
  .app-header {
    position: sticky;
    z-index: 20;
    top: 0;
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    align-items: center;
    min-height: 52px;
    padding: 0 24px;
    background: color-mix(in srgb, var(--paper) 97%, transparent);
    border-bottom: 1px solid var(--rule);
    backdrop-filter: blur(10px);
  }
  .product-context { display: flex; align-self: stretch; align-items: center; gap: 12px; min-width: 0; padding-right: 24px; border-right: 1px solid var(--rule); }
  .product-context strong { color: var(--graphite); font-size: 16px; letter-spacing: -.02em; }
  .product-context span { overflow: hidden; color: var(--pencil); font: 11px var(--mono); text-overflow: ellipsis; white-space: nowrap; }
  nav { display: flex; align-self: stretch; gap: 2px; margin-left: 8px; }
  nav button { position: relative; min-width: 64px; padding: 0 14px; color: var(--pencil); background: transparent; font-size: 13px; }
  nav button:hover { color: var(--graphite); background: var(--paper-shade); }
  nav button.active { color: var(--graphite); font-weight: 600; }
  nav button.active::after { position: absolute; right: 12px; bottom: -1px; left: 12px; height: 2px; background: var(--selection); content: ''; }
  @media (max-width: 1050px) {
    .app-header { padding-inline: 20px; }
    .product-context { padding-right: 18px; }
  }
</style>

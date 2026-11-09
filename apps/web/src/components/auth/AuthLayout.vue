<script setup lang="ts">
import { useId } from 'vue'

import AppBrand from '@/components/layout/AppBrand.vue'
import AppSurface from '@/components/ui/AppSurface.vue'

withDefaults(
  defineProps<{
    title: string
    description?: string
    eyebrow?: string
  }>(),
  {
    description: '',
    eyebrow: 'KAPOJ // ACCESS',
  },
)

defineSlots<{
  default(): unknown
  footer(): unknown
}>()

const titleId = useId()
</script>

<template>
  <div class="auth-layout">
    <aside class="auth-intro">
      <AppBrand class="auth-brand" />

      <div class="auth-intro__copy">
        <p class="auth-intro__kicker">KAPOJ // ONLINE JUDGE</p>
        <h1 class="auth-intro__title">
          把每一次提交<br />
          <span>变成成长的坐标。</span>
        </h1>
        <p class="auth-intro__description">
          实时评测、竞赛榜单、成就与徽章，构建属于你的竞赛档案。
        </p>
      </div>

      <ul class="auth-intro__features">
        <li><i class="pi pi-bolt" aria-hidden="true" /> 多语言实时评测</li>
        <li><i class="pi pi-trophy" aria-hidden="true" /> ICPC / IOI / OI 赛制</li>
        <li><i class="pi pi-star" aria-hidden="true" /> 成就、任务与装扮</li>
      </ul>

      <div class="auth-intro__status">
        <span class="auth-intro__pulse" aria-hidden="true" />
        <span>系统在线</span>
        <span class="auth-intro__version">M4-C</span>
      </div>
    </aside>

    <AppSurface
      as="section"
      tone="raised"
      padding="lg"
      class="auth-panel"
      :aria-labelledby="titleId"
    >
      <header class="auth-panel__head">
        <p class="auth-panel__eyebrow">{{ eyebrow }}</p>
        <h2 :id="titleId" class="auth-panel__title">{{ title }}</h2>
        <p v-if="description" class="auth-panel__description">{{ description }}</p>
      </header>
      <slot />
      <div v-if="$slots.footer" class="auth-panel__footer">
        <slot name="footer" />
      </div>
    </AppSurface>
  </div>
</template>

<style scoped>
.auth-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(360px, 420px);
  gap: var(--kapoj-space-8);
  align-items: center;
  width: 100%;
  max-width: 1080px;
  min-height: calc(100vh - var(--kapoj-header-height) - 2 * var(--kapoj-space-5));
  margin: 0 auto;
}

.auth-intro {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
  min-width: 0;
}

.auth-brand {
  align-self: flex-start;
}

.auth-intro__copy {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.auth-intro__kicker {
  margin: 0;
  color: var(--kapoj-primary);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.16em;
}

.auth-intro__title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: clamp(2.1rem, 1.4rem + 2.8vw, 3.4rem);
  line-height: 1.05;
  letter-spacing: -0.045em;
}

.auth-intro__title span {
  color: var(--kapoj-primary);
}

.auth-intro__description {
  max-width: 44ch;
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-size: 14px;
}

.auth-intro__features {
  display: grid;
  gap: var(--kapoj-space-2);
  padding: 0;
  margin: 0;
  list-style: none;
}

.auth-intro__features li {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  color: var(--kapoj-text-secondary);
  font-size: 13px;
}

.auth-intro__features i {
  width: 16px;
  color: var(--kapoj-primary);
  font-size: 13px;
  text-align: center;
}

.auth-intro__status {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  letter-spacing: 0.08em;
}

.auth-intro__pulse {
  width: 6px;
  height: 6px;
  background: var(--kapoj-success);
  border-radius: 50%;
  box-shadow: 0 0 8px var(--kapoj-success);
}

.auth-intro__version {
  margin-left: auto;
  color: var(--kapoj-text-secondary);
}

.auth-panel {
  position: relative;
  width: 100%;
  overflow: hidden;
}

.auth-panel::before {
  position: absolute;
  inset: 0 0 auto;
  height: 1px;
  content: '';
  background: linear-gradient(90deg, transparent, var(--kapoj-primary), transparent);
  opacity: 0.8;
}

.auth-panel__head {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-1);
  margin-bottom: var(--kapoj-space-5);
}

.auth-panel__eyebrow {
  margin: 0;
  color: var(--kapoj-primary);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.auth-panel__title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1.5rem;
  line-height: 1.2;
  letter-spacing: -0.02em;
}

.auth-panel__description {
  margin: var(--kapoj-space-1) 0 0;
  color: var(--kapoj-text-secondary);
  font-size: 13px;
}

.auth-panel__footer {
  margin-top: var(--kapoj-space-4);
  padding-top: var(--kapoj-space-4);
  color: var(--kapoj-text-secondary);
  font-size: 13px;
  text-align: center;
  border-top: 1px solid var(--kapoj-border);
}

.auth-panel__footer :deep(a) {
  color: var(--kapoj-primary);
  font-weight: 600;
}

@media (max-width: 900px) {
  .auth-layout {
    grid-template-columns: minmax(0, 1fr);
    max-width: 440px;
    min-height: auto;
    margin-top: var(--kapoj-space-4);
  }

  .auth-intro {
    display: none;
  }
}

@media (max-width: 600px) {
  .auth-layout {
    margin-top: 0;
  }
}
</style>

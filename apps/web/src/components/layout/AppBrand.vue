<script setup lang="ts">
import { computed, ref, watch } from 'vue'

import { useSiteStore } from '@/stores/site'

withDefaults(
  defineProps<{
    badge?: string
    drawer?: boolean
  }>(),
  {
    badge: '',
    drawer: false,
  },
)

const site = useSiteStore()
const logoFailed = ref(false)
const showLogo = computed(() => Boolean(site.logoUrl) && !logoFailed.value)

watch(
  () => site.logoUrl,
  () => {
    logoFailed.value = false
  },
)
</script>

<template>
  <RouterLink
    to="/"
    class="app-brand"
    :class="{ 'app-brand--drawer': drawer }"
    aria-label="kapOJ 首页"
  >
    <img
      v-if="showLogo"
      class="app-brand__logo"
      :src="site.logoUrl"
      alt="kapOJ 首页"
      @error="logoFailed = true"
    />
    <template v-else>
      <span class="app-brand__mark" aria-hidden="true">K</span>
      <span class="app-brand__name">kap<strong>OJ</strong></span>
    </template>
    <span v-if="badge" class="app-brand__badge">{{ badge }}</span>
  </RouterLink>
</template>

<style scoped>
.app-brand {
  display: flex;
  flex: none;
  align-items: center;
  gap: var(--kapoj-space-2);
  padding: 0 var(--kapoj-space-1);
  color: var(--kapoj-text);
  text-decoration: none;
  border-radius: var(--kapoj-radius-sm);
}

.app-brand:hover {
  text-decoration: none;
}

.app-brand:focus-visible {
  outline: 2px solid var(--kapoj-border-focus);
  outline-offset: 3px;
}

.app-brand__logo {
  display: block;
  width: auto;
  max-width: 180px;
  height: 32px;
  object-fit: contain;
}

.app-brand--drawer .app-brand__logo {
  height: 34px;
}

.app-brand__mark {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  color: var(--kapoj-text-inverted);
  font-family: var(--kapoj-font-mono);
  font-size: 16px;
  font-weight: 700;
  background: var(--kapoj-brand);
  border-radius: var(--kapoj-radius-md);
  box-shadow: 0 4px 12px color-mix(in srgb, var(--kapoj-brand) 24%, transparent);
}

.app-brand__name {
  font-family: var(--kapoj-font-mono);
  font-size: 19px;
  font-weight: 700;
  letter-spacing: -0.045em;
}

.app-brand__name strong {
  color: var(--kapoj-brand);
  font-weight: inherit;
}

.app-brand__badge {
  padding: 1px 6px;
  color: var(--kapoj-primary);
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
  font-weight: 600;
  letter-spacing: 0.06em;
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-brand) 22%, transparent);
  border-radius: 999px;
}
</style>

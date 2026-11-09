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
}

.app-brand:hover {
  text-decoration: none;
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
  width: 32px;
  height: 32px;
  color: var(--kapoj-bg);
  font-family: var(--kapoj-font-mono);
  font-size: 15px;
  font-weight: 700;
  background: var(--kapoj-primary);
  border-radius: var(--kapoj-radius-sm);
  box-shadow: var(--kapoj-glow-primary);
}

.app-brand__name {
  font-family: var(--kapoj-font-mono);
  font-size: 18px;
  font-weight: 700;
  letter-spacing: -0.04em;
}

.app-brand__name strong {
  color: var(--kapoj-primary);
  font-weight: inherit;
}

.app-brand__badge {
  padding: 1px var(--kapoj-space-1);
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
  letter-spacing: 0.08em;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-xs);
}
</style>

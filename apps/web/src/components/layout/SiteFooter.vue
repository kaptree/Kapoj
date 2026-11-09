<script setup lang="ts">
import { computed } from 'vue'

import { useSiteStore } from '@/stores/site'

const site = useSiteStore()

const hasFiling = computed(() => Boolean(site.footer.icpNumber || site.footer.policeNumber))
const hasPoliceLink = computed(() => Boolean(site.footer.policeNumber && site.footer.policeUrl))
const formattedVisits = computed(() => new Intl.NumberFormat('zh-CN').format(site.visitCount))
</script>

<template>
  <footer class="site-footer">
    <p v-if="site.footer.copyright" class="site-footer__copyright">
      {{ site.footer.copyright }}
    </p>

    <p v-if="hasFiling" class="site-footer__filing">
      <a
        v-if="site.footer.icpNumber"
        class="site-footer__icp"
        href="https://beian.miit.gov.cn/"
        target="_blank"
        rel="noopener noreferrer"
      >
        {{ site.footer.icpNumber }}
      </a>
      <span
        v-if="site.footer.icpNumber && site.footer.policeNumber"
        class="site-footer__divider"
        aria-hidden="true"
      >
        ·
      </span>
      <a
        v-if="hasPoliceLink"
        class="site-footer__police"
        :href="site.footer.policeUrl"
        target="_blank"
        rel="noopener noreferrer"
      >
        {{ site.footer.policeNumber }}
      </a>
      <span v-else-if="site.footer.policeNumber" class="site-footer__police-text">
        {{ site.footer.policeNumber }}
      </span>
    </p>

    <p v-if="site.footer.text" class="site-footer__text">{{ site.footer.text }}</p>

    <p class="site-footer__meta">
      <span>本站已运行 {{ site.runningDays }} 天</span>
      <template v-if="site.footer.showVisits">
        <span class="site-footer__divider" aria-hidden="true"> · </span>
        <span>总访问量 {{ formattedVisits }}</span>
      </template>
    </p>
  </footer>
</template>

<style scoped>
.site-footer {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-1);
  padding-top: var(--kapoj-space-5);
  margin-top: var(--kapoj-space-6);
  color: var(--kapoj-text-muted);
  font-size: 11px;
  line-height: 1.6;
  border-top: 1px solid var(--kapoj-border);
}

.site-footer p {
  margin: 0;
}

.site-footer a {
  color: var(--kapoj-text-secondary);
  text-decoration: none;
}

.site-footer a:hover {
  color: var(--kapoj-primary);
  text-decoration: underline;
}

.site-footer__text {
  white-space: pre-line;
}

.site-footer__divider {
  color: var(--kapoj-text-muted);
}

@media (max-width: 600px) {
  .site-footer {
    gap: var(--kapoj-space-2);
    font-size: 10px;
  }
}
</style>

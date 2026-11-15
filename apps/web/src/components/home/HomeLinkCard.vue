<script setup lang="ts">
import { computed } from 'vue'

import type { SiteLink } from '@/api/types'
import AppSurface from '@/components/ui/AppSurface.vue'

type HomeLinkTone = 'teal' | 'amber' | 'blue' | 'violet' | 'green' | 'rose'

const props = withDefaults(
  defineProps<{
    link: SiteLink
    tone?: HomeLinkTone
    meta?: string
    badge?: string
    featured?: boolean
  }>(),
  {
    tone: 'teal',
    meta: '',
    badge: '',
    featured: false,
  },
)

const badgeText = computed(() => props.badge.trim())

const accessibleLabel = computed(() => {
  const parts = [props.link.title]
  if (badgeText.value) parts.push(badgeText.value)
  if (props.link.description) parts.push(props.link.description)
  return parts.join('，')
})
</script>

<template>
  <RouterLink
    :to="link.to"
    class="home-link-card"
    :class="[`home-link-card--${tone}`, { 'is-featured': featured }]"
    :aria-label="accessibleLabel"
    :data-target="link.to"
  >
    <AppSurface as="article" padding="none" class="home-link-card__surface">
      <div class="home-link-card__head">
        <span v-if="link.icon" class="home-link-card__icon">
          <i :class="link.icon" aria-hidden="true" />
        </span>
        <span v-if="badgeText" class="home-link-card__badge">{{ badgeText }}</span>
        <span class="home-link-card__arrow" aria-hidden="true">
          <i class="pi pi-arrow-up-right" />
        </span>
      </div>

      <div class="home-link-card__body">
        <p v-if="meta" class="home-link-card__meta">{{ meta }}</p>
        <h3 class="home-link-card__title">{{ link.title }}</h3>
        <p v-if="link.description" class="home-link-card__description">{{ link.description }}</p>
      </div>
    </AppSurface>
  </RouterLink>
</template>

<style scoped>
.home-link-card {
  --card-accent: var(--kapoj-brand);
  --home-card-min-height: 156px;

  display: block;
  height: 100%;
  color: inherit;
  text-decoration: none;
  border-radius: var(--kapoj-radius-md);
}

.home-link-card:hover,
.home-link-card:focus-visible {
  text-decoration: none;
}

.home-link-card:focus-visible {
  outline: 2px solid var(--kapoj-border-focus);
  outline-offset: 3px;
}

.home-link-card--amber {
  --card-accent: var(--kapoj-warning);
}

.home-link-card--blue {
  --card-accent: var(--kapoj-info);
}

.home-link-card--violet {
  --card-accent: var(--kapoj-special);
}

.home-link-card--green {
  --card-accent: var(--kapoj-success);
}

.home-link-card--rose {
  --card-accent: var(--kapoj-re);
}

.home-link-card__surface {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-height: var(--home-card-min-height);
  padding: var(--kapoj-space-4);
  overflow: hidden;
  box-shadow: var(--kapoj-shadow-sm);
  transition:
    transform var(--kapoj-transition),
    box-shadow var(--kapoj-transition),
    border-color var(--kapoj-transition);
}

.home-link-card.is-featured .home-link-card__surface {
  border-color: color-mix(in srgb, var(--card-accent) 38%, var(--kapoj-border));
}

.home-link-card.is-featured .home-link-card__surface::before {
  position: absolute;
  inset: 0 0 auto;
  height: 2px;
  content: '';
  background: var(--card-accent);
}

.home-link-card:hover .home-link-card__surface {
  border-color: color-mix(in srgb, var(--card-accent) 34%, var(--kapoj-border));
  box-shadow: var(--kapoj-shadow-hover);
  transform: translateY(-3px);
}

.home-link-card__head {
  display: flex;
  align-items: flex-start;
  gap: var(--kapoj-space-2);
}

.home-link-card__icon {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  color: var(--card-accent);
  font-size: 16px;
  background: color-mix(in srgb, var(--card-accent) 11%, var(--kapoj-surface));
  border: 1px solid color-mix(in srgb, var(--card-accent) 26%, transparent);
  border-radius: var(--kapoj-radius-sm);
  transition:
    background var(--kapoj-transition),
    border-color var(--kapoj-transition);
}

.home-link-card:hover .home-link-card__icon {
  background: color-mix(in srgb, var(--card-accent) 17%, var(--kapoj-surface));
  border-color: color-mix(in srgb, var(--card-accent) 38%, transparent);
}

.home-link-card__badge {
  align-self: center;
  max-width: 12rem;
  padding: 2px var(--kapoj-space-2);
  margin-left: auto;
  overflow: hidden;
  color: var(--card-accent);
  font-size: 10px;
  font-weight: 600;
  line-height: 1.5;
  text-overflow: ellipsis;
  white-space: nowrap;
  background: color-mix(in srgb, var(--card-accent) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--card-accent) 24%, transparent);
  border-radius: 999px;
}

.home-link-card__arrow {
  flex: none;
  padding-top: 2px;
  margin-left: auto;
  color: var(--kapoj-text-muted);
  font-size: 12px;
  transition:
    color var(--kapoj-transition),
    transform var(--kapoj-transition);
}

.home-link-card__badge + .home-link-card__arrow {
  margin-left: 0;
}

.home-link-card:hover .home-link-card__arrow {
  color: var(--card-accent);
  transform: translate(2px, -2px);
}

.home-link-card__body {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
}

.home-link-card__meta {
  margin: 0 0 3px;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
  font-weight: 600;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.home-link-card__title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1rem;
  font-weight: 650;
  line-height: 1.35;
  letter-spacing: -0.01em;
  overflow-wrap: anywhere;
}

.home-link-card__description {
  margin: var(--kapoj-space-1) 0 0;
  color: var(--kapoj-text-secondary);
  font-size: 12.5px;
  line-height: 1.55;
  overflow-wrap: anywhere;
}

@media (max-width: 640px) {
  .home-link-card {
    --home-card-min-height: 146px;
  }

  .home-link-card__surface {
    gap: var(--kapoj-space-2);
  }
}

@media (prefers-reduced-motion: reduce) {
  .home-link-card__surface,
  .home-link-card__icon,
  .home-link-card__arrow {
    transition: none;
  }

  .home-link-card:hover .home-link-card__surface {
    transform: none;
  }

  .home-link-card:hover .home-link-card__arrow {
    transform: none;
  }
}
</style>

<script setup lang="ts">
import { computed, useId } from 'vue'

interface FieldSlotProps {
  id: string
  invalid: boolean
  describedby: string | undefined
}

const props = defineProps<{
  label: string
  error?: string
  hint?: string
  optional?: boolean
  required?: boolean
}>()

defineSlots<{
  default(props: FieldSlotProps): unknown
}>()

const id = useId()
const errorId = `${id}-error`
const hintId = `${id}-hint`
const invalid = computed(() => Boolean(props.error))
const describedby = computed(() => {
  if (props.error) return errorId
  if (props.hint) return hintId
  return undefined
})
</script>

<template>
  <div class="app-field" :class="{ 'app-field--invalid': invalid }">
    <label class="app-field__label" :for="id">
      <span>{{ label }}</span>
      <span v-if="optional" class="app-field__optional">可选</span>
      <span v-else-if="required" class="app-field__required" aria-hidden="true">*</span>
    </label>
    <slot :id="id" :invalid="invalid" :describedby="describedby" />
    <p v-if="error" :id="errorId" class="app-field__error" role="alert">{{ error }}</p>
    <p v-else-if="hint" :id="hintId" class="app-field__hint">{{ hint }}</p>
  </div>
</template>

<style scoped>
.app-field {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  min-width: 0;
}

.app-field__label {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-1);
  color: var(--kapoj-text-secondary);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
}

.app-field__required {
  color: var(--kapoj-danger);
}

.app-field__optional {
  padding: 0 var(--kapoj-space-1);
  color: var(--kapoj-text-muted);
  font-size: 10px;
  font-weight: 500;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-xs);
}

.app-field__error,
.app-field__hint {
  margin: 0;
  font-size: 12px;
  line-height: 1.45;
}

.app-field__error {
  color: var(--kapoj-danger);
}

.app-field__hint {
  color: var(--kapoj-text-muted);
}

.app-field :deep(input),
.app-field :deep(textarea),
.app-field :deep(.p-password),
.app-field :deep(.p-password-input) {
  width: 100%;
}

.app-field :deep(.p-inputtext),
.app-field :deep(.p-textarea),
.app-field :deep(.p-password-input) {
  background: var(--kapoj-bg);
  border-color: var(--kapoj-border-strong);
  border-radius: var(--kapoj-radius-sm);
  transition:
    border-color var(--kapoj-transition),
    box-shadow var(--kapoj-transition);
}

.app-field :deep(.p-inputtext:hover),
.app-field :deep(.p-textarea:hover),
.app-field :deep(.p-password-input:hover) {
  border-color: var(--kapoj-text-muted);
}

.app-field :deep(.p-inputtext:focus),
.app-field :deep(.p-textarea:focus),
.app-field :deep(.p-password-input:focus) {
  border-color: var(--kapoj-primary);
  box-shadow: 0 0 0 3px var(--kapoj-primary-soft);
}

.app-field :deep(.p-inputtext.p-invalid),
.app-field :deep(.p-textarea.p-invalid),
.app-field :deep(.p-password-input.p-invalid) {
  border-color: var(--kapoj-danger);
}

.app-field :deep(.p-inputtext.p-invalid:focus),
.app-field :deep(.p-textarea.p-invalid:focus),
.app-field :deep(.p-password-input.p-invalid:focus) {
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--kapoj-danger) 18%, transparent);
}

@media (prefers-reduced-motion: reduce) {
  .app-field :deep(.p-inputtext),
  .app-field :deep(.p-textarea),
  .app-field :deep(.p-password-input) {
    transition: none;
  }
}
</style>

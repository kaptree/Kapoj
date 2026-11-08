<script setup lang="ts">
import Button from 'primevue/button'
import Textarea from 'primevue/textarea'

import type { Sample } from '@/api/types'

const samples = defineModel<Sample[]>({ required: true })

const MAX_SAMPLES = 10

function add() {
  if (samples.value.length < MAX_SAMPLES)
    samples.value = [...samples.value, { input: '', output: '' }]
}

function remove(index: number) {
  samples.value = samples.value.filter((_, i) => i !== index)
}

function update(index: number, field: 'input' | 'output', value: string) {
  samples.value = samples.value.map((sample, i) =>
    i === index ? { ...sample, [field]: value } : sample,
  )
}
</script>

<template>
  <div class="sample-editor">
    <p v-if="samples.length === 0" class="hint">还没有样例。</p>
    <section v-for="(sample, i) in samples" :key="i" class="sample">
      <header class="sample-head">
        <span class="sample-title">样例 {{ i + 1 }}</span>
        <Button label="删除" text severity="danger" size="small" @click="remove(i)" />
      </header>
      <div class="sample-grid">
        <label class="sample-field">
          <span class="sample-field__label">输入</span>
          <Textarea
            :model-value="sample.input"
            rows="4"
            placeholder="输入"
            class="mono"
            @update:model-value="update(i, 'input', $event)"
          />
        </label>
        <label class="sample-field">
          <span class="sample-field__label">输出</span>
          <Textarea
            :model-value="sample.output"
            rows="4"
            placeholder="输出"
            class="mono"
            @update:model-value="update(i, 'output', $event)"
          />
        </label>
      </div>
    </section>
    <div class="sample-actions">
      <Button
        label="添加样例"
        icon="pi pi-plus"
        :disabled="samples.length >= MAX_SAMPLES"
        @click="add"
      />
      <span v-if="samples.length >= MAX_SAMPLES" class="hint">最多 {{ MAX_SAMPLES }} 组样例。</span>
    </div>
  </div>
</template>

<style scoped>
.sample-editor {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.sample {
  padding: var(--kapoj-space-3);
  background: var(--kapoj-surface);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.sample-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--kapoj-space-3);
}

.sample-title {
  color: var(--kapoj-text);
  font-size: 13px;
  font-weight: 600;
}

.sample-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--kapoj-space-3);
}

.sample-field {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  min-width: 0;
}

.sample-field__label {
  color: var(--kapoj-text-secondary);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
}

.mono {
  width: 100%;
  font-family: var(--kapoj-font-mono);
  background: var(--kapoj-bg);
  border-color: var(--kapoj-border-strong);
  border-radius: var(--kapoj-radius-sm);
}

.sample-actions {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-3);
}

.hint {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

@media (max-width: 640px) {
  .sample-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>

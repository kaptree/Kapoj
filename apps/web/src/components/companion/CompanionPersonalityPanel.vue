<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'

const props = defineProps<{
  personalityPrompt: string
  aiDialogueEnabled: boolean
  aiReady: boolean
  remainingToday: number
}>()

const emit = defineEmits<{
  save: [patch: { personalityPrompt: string; aiDialogueEnabled: boolean }]
  reset: []
}>()

const prompt = ref(props.personalityPrompt)
const aiEnabled = ref(props.aiDialogueEnabled)
const error = ref('')

watch(
  () => props.personalityPrompt,
  (value) => {
    prompt.value = value
  },
)

watch(
  () => props.aiDialogueEnabled,
  (value) => {
    aiEnabled.value = value
  },
)

const count = computed(() => Array.from(prompt.value).length)

function onInput(event: Event): void {
  prompt.value = (event.target as HTMLTextAreaElement).value
  error.value = ''
}

function save(): void {
  error.value = ''
  const trimmed = prompt.value.trim()
  if (Array.from(trimmed).length > 500) {
    error.value = '性格提示词不能超过 500 个字符'
    return
  }
  emit('save', { personalityPrompt: trimmed, aiDialogueEnabled: aiEnabled.value })
}

function reset(): void {
  prompt.value = ''
  aiEnabled.value = props.aiDialogueEnabled
  error.value = ''
  emit('reset')
  emit('save', { personalityPrompt: '', aiDialogueEnabled: aiEnabled.value })
}

function toggleAI(): void {
  if (!props.aiReady) return
  aiEnabled.value = !aiEnabled.value
}
</script>

<template>
  <section class="companion-personality" aria-labelledby="companion-personality-title">
    <div class="companion-personality__head">
      <span id="companion-personality-title" class="companion-personality__title">性格与 AI 台词</span>
      <span class="companion-personality__hint">留空使用系统默认的傲娇可爱性格。</span>
    </div>

    <label class="companion-personality__label" for="companion-personality-prompt">
      性格提示词
    </label>
    <textarea
      id="companion-personality-prompt"
      data-test="personality-prompt"
      class="companion-personality__textarea"
      maxlength="500"
      rows="4"
      :value="prompt"
      placeholder="例如：更温柔一点，会鼓励我，但偶尔还是会嘴硬。"
      @input="onInput"
    />
    <div class="companion-personality__meta">
      <span class="companion-personality__hint">最多 500 字，只影响语气，不会覆盖安全规则。</span>
      <span data-test="personality-count" class="companion-personality__count">{{ count }}/500</span>
    </div>

    <div class="companion-personality__ai">
      <div class="companion-personality__copy">
        <span class="companion-personality__label">AI 生成互动台词</span>
        <span class="companion-personality__hint">
          <template v-if="aiReady">
            已接入 AI；抚摸、问候和活泼模式的闲话会由 AI 生成，失败自动回退默认台词。
            今日剩余 {{ remainingToday }} 次。
          </template>
          <template v-else>
            尚未接入 AI，所有互动使用默认台词。先去
            <RouterLink to="/settings/ai" class="companion-personality__link">AI 设置</RouterLink>
            配置模型。
          </template>
        </span>
      </div>
      <button
        type="button"
        class="companion-personality__toggle"
        data-test="ai-dialogue-toggle"
        :aria-pressed="aiEnabled"
        :disabled="!aiReady"
        @click="toggleAI"
      >
        {{ aiEnabled ? 'AI 台词已开启' : 'AI 台词已关闭' }}
      </button>
    </div>

    <p v-if="error" class="companion-personality__error" role="status">{{ error }}</p>

    <div class="companion-personality__actions">
      <button
        type="button"
        class="companion-personality__reset"
        data-test="personality-reset"
        @click="reset"
      >
        恢复默认
      </button>
      <button
        type="button"
        class="companion-personality__save"
        data-test="personality-save"
        @click="save"
      >
        保存性格
      </button>
    </div>
  </section>
</template>

<style scoped>
.companion-personality {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding-top: var(--kapoj-space-3);
  border-top: 1px solid var(--kapoj-border);
}

.companion-personality__head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: var(--kapoj-space-2);
}

.companion-personality__title {
  color: var(--kapoj-text);
  font-size: 0.95rem;
  font-weight: 600;
}

.companion-personality__label {
  color: var(--kapoj-text);
  font-size: 0.85rem;
  font-weight: 600;
}

.companion-personality__hint {
  color: var(--kapoj-text-muted);
  font-size: 12px;
  line-height: 1.5;
}

.companion-personality__textarea {
  box-sizing: border-box;
  width: 100%;
  padding: 8px 10px;
  font-family: inherit;
  font-size: 0.86rem;
  line-height: 1.5;
  color: var(--kapoj-text);
  resize: vertical;
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.companion-personality__textarea:focus-visible {
  outline: 2px solid var(--kapoj-kapo);
  outline-offset: 1px;
}

.companion-personality__meta,
.companion-personality__ai,
.companion-personality__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
}

.companion-personality__count {
  color: var(--kapoj-text-muted);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}

.companion-personality__copy {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.companion-personality__link {
  color: var(--kapoj-kapo-text);
}

.companion-personality__toggle,
.companion-personality__reset,
.companion-personality__save {
  padding: 6px 12px;
  font-size: 0.82rem;
  color: var(--kapoj-text);
  cursor: pointer;
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.companion-personality__toggle[aria-pressed='true'] {
  color: var(--kapoj-kapo-text);
  background: var(--kapoj-kapo-soft);
  border-color: color-mix(in srgb, var(--kapoj-kapo) 45%, var(--kapoj-border));
}

.companion-personality__toggle:disabled {
  cursor: default;
  opacity: 0.55;
}

.companion-personality__save {
  color: var(--kapoj-kapo-text);
  background: var(--kapoj-kapo-soft);
  border-color: color-mix(in srgb, var(--kapoj-kapo) 45%, var(--kapoj-border));
}

.companion-personality__reset {
  background: transparent;
}

.companion-personality__toggle:hover:not(:disabled),
.companion-personality__reset:hover,
.companion-personality__save:hover {
  border-color: var(--kapoj-border-strong);
}

.companion-personality__toggle:focus-visible,
.companion-personality__reset:focus-visible,
.companion-personality__save:focus-visible {
  outline: 2px solid var(--kapoj-kapo);
  outline-offset: 1px;
}

.companion-personality__error {
  margin: 0;
  color: var(--kapoj-kapo-text);
  font-size: 12px;
}

@media (max-width: 640px) {
  .companion-personality__ai,
  .companion-personality__actions {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>

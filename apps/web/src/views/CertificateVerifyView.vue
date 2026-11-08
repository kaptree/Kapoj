<script setup lang="ts">
import { onMounted, ref } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { useRoute } from 'vue-router'

import { verifyCertificate } from '@/api/certificates'
import { ApiError } from '@/api/http'
import type { PublicCertificateView } from '@/api/types'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { formatDateTime } from '@/utils/format'

const route = useRoute()

const code = ref('')
const loading = ref(false)
const result = ref<PublicCertificateView | null>(null)
const error = ref('')

async function verify() {
  const trimmed = code.value.trim()
  if (!trimmed || loading.value) return

  loading.value = true
  error.value = ''
  result.value = null
  try {
    result.value = await verifyCertificate(trimmed)
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '核验失败，请稍后再试'
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  const queryCode = route.query.code
  if (typeof queryCode === 'string' && queryCode.trim() !== '') {
    code.value = queryCode
    void verify()
  }
})
</script>

<template>
  <div class="verify-page">
    <AppPageHeader
      eyebrow="VERIFY // CERTIFICATE"
      title="证书核验"
      description="输入验证码核验证书的类型、等级、持有者昵称与通过日期；核验结果不展示证书图片与任何隐私信息。"
    />

    <AppSurface padding="lg" class="verify-surface">
      <form class="verify-form" novalidate @submit.prevent="verify">
        <InputText
          v-model="code"
          class="code-input"
          placeholder="KAPOJ-XXXX-XXXX"
          aria-label="验证码"
          fluid
          autocomplete="off"
          @update:model-value="error = ''"
        />
        <Button
          type="submit"
          label="核验"
          icon="pi pi-search"
          :loading="loading"
          :disabled="loading"
        />
      </form>
      <p class="format-hint">验证码格式：KAPOJ-XXXX-XXXX（不区分大小写，空格自动去除）</p>

      <Message v-if="error" severity="error" :closable="false" class="verify-error" role="alert">
        {{ error }}
      </Message>

      <div v-if="result" class="result-card">
        <template v-if="result.valid">
          <div class="result-valid">
            <Tag value="验证有效" severity="success" />
            <dl class="result-list">
              <div class="result-row">
                <dt>证书类型</dt>
                <dd>{{ result.type }}</dd>
              </div>
              <div class="result-row">
                <dt>等级</dt>
                <dd>{{ result.level }}</dd>
              </div>
              <div class="result-row">
                <dt>持有者</dt>
                <dd>{{ result.nickname }}</dd>
              </div>
              <div class="result-row">
                <dt>通过日期</dt>
                <dd>{{ result.approvedAt ? formatDateTime(result.approvedAt) : '—' }}</dd>
              </div>
            </dl>
          </div>
        </template>
        <Message v-else severity="secondary" :closable="false" role="status">
          未找到有效证书，请检查验证码
        </Message>
      </div>
    </AppSurface>
  </div>
</template>

<style scoped>
.verify-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
  max-width: var(--kapoj-content-max);
}

.verify-surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.verify-form {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
}

.verify-form :deep(.p-inputtext) {
  flex: 1 1 220px;
  font-family: var(--kapoj-font-mono);
  text-transform: uppercase;
}

.format-hint {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.verify-error {
  margin: 0;
}

.result-card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  padding: var(--kapoj-space-4);
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.result-valid {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.result-valid :deep(.p-tag) {
  align-self: flex-start;
}

.result-list {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  margin: 0;
}

.result-row {
  display: flex;
  gap: var(--kapoj-space-3);
}

.result-row dt {
  flex: 0 0 72px;
  color: var(--kapoj-text-muted);
  font-size: 13px;
}

.result-row dd {
  margin: 0;
  color: var(--kapoj-text);
  font-weight: 600;
}

@media (max-width: 640px) {
  .verify-form :deep(.p-button) {
    flex: 1 1 100%;
  }
}
</style>

<script setup lang="ts">
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
</script>

<template>
  <div>
    <p v-if="samples.length === 0" class="hint">还没有样例。</p>
    <el-card v-for="(sample, i) in samples" :key="i" shadow="never" class="sample">
      <template #header>
        <div class="head">
          <span>样例 {{ i + 1 }}</span>
          <el-button text type="danger" @click="remove(i)">删除</el-button>
        </div>
      </template>
      <el-row :gutter="12">
        <el-col :span="12">
          <el-input
            v-model="sample.input"
            type="textarea"
            :rows="4"
            placeholder="输入"
            class="mono"
          />
        </el-col>
        <el-col :span="12">
          <el-input
            v-model="sample.output"
            type="textarea"
            :rows="4"
            placeholder="输出"
            class="mono"
          />
        </el-col>
      </el-row>
    </el-card>
    <el-button :disabled="samples.length >= MAX_SAMPLES" @click="add">添加样例</el-button>
    <span v-if="samples.length >= MAX_SAMPLES" class="hint">最多 {{ MAX_SAMPLES }} 组样例。</span>
  </div>
</template>

<style scoped>
.sample {
  margin-bottom: 12px;
}
.head {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.hint {
  color: var(--el-text-color-secondary);
}
.mono :deep(textarea) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}
</style>

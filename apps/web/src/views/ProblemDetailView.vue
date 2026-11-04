<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'

import { ApiError } from '@/api/http'
import { getProblem, getStatistics } from '@/api/problems'
import type { ProblemDetail, ProblemStats } from '@/api/types'
import MarkdownView from '@/components/MarkdownView.vue'
import { hasRole } from '@/router/guard'
import { useAuthStore } from '@/stores/auth'
import { difficultyLabel } from '@/utils/difficulty'

const route = useRoute()
const auth = useAuthStore()
const canManage = computed(() => hasRole(auth.user?.role ?? null, ['problem_admin']))

const problem = ref<ProblemDetail | null>(null)
const stats = ref<ProblemStats | null>(null)
const loading = ref(false)
const error = ref('')

const typeLabels = { traditional: '传统题', special: '特判题', interactive: '交互题' } as const

let latest = 0

watch(
  () => route.params.id,
  async (id) => {
    if (typeof id !== 'string') return
    const seq = ++latest
    loading.value = true
    error.value = ''
    problem.value = null
    stats.value = null
    try {
      const detail = await getProblem(id)
      if (seq !== latest) return
      problem.value = detail
      // Statistics are a nice-to-have; the page works without them, but a
      // late response for a previous problem must not overwrite this one.
      const nextStats = await getStatistics(id).catch(() => null)
      if (seq !== latest) return
      stats.value = nextStats
    } catch (e) {
      if (seq !== latest) return
      error.value =
        e instanceof ApiError && e.code === 40400 ? '题目不存在' : '加载题目失败，请稍后再试'
    } finally {
      if (seq === latest) loading.value = false
    }
  },
  { immediate: true },
)

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制')
  } catch {
    ElMessage.error('复制失败，请手动选择文本')
  }
}

const rate = computed(() => (stats.value ? `${(stats.value.acceptRate * 100).toFixed(1)}%` : '-'))
</script>

<template>
  <div v-loading="loading" class="page">
    <el-result v-if="error" icon="warning" :title="error">
      <template #extra>
        <RouterLink :to="{ name: 'problems' }"
          ><el-button type="primary">返回题库</el-button></RouterLink
        >
      </template>
    </el-result>

    <el-row v-else-if="problem" :gutter="16">
      <el-col :xs="24" :md="16">
        <el-card shadow="never">
          <el-alert
            v-if="!problem.isPublic"
            title="这道题目尚未公开，只有题目管理员能看到。"
            type="warning"
            show-icon
            :closable="false"
            class="banner"
          />
          <div class="head">
            <h1>{{ problem.title }}</h1>
            <div class="actions">
              <RouterLink :to="{ name: 'problem-submit', params: { id: problem.id } }">
                <el-button type="primary">提交代码</el-button>
              </RouterLink>
              <RouterLink
                v-if="canManage"
                :to="{ name: 'problem-edit', params: { id: problem.id } }"
              >
                <el-button>编辑</el-button>
              </RouterLink>
            </div>
          </div>
          <div class="meta">
            <el-tag>{{ difficultyLabel(problem.difficulty) }}</el-tag>
            <el-tag type="info">{{ typeLabels[problem.type] }}</el-tag>
            <span>时间限制 {{ problem.timeLimitMs }} ms</span>
            <span>内存限制 {{ problem.memoryLimitMb }} MB</span>
            <span v-if="problem.source">来源：{{ problem.source }}</span>
          </div>
          <div class="tags">
            <el-tag v-for="t in problem.tags" :key="t" size="small" effect="plain">{{ t }}</el-tag>
          </div>

          <h2>题目描述</h2>
          <MarkdownView :source="problem.statementMd" />
          <template v-if="problem.inputFormatMd">
            <h2>输入格式</h2>
            <MarkdownView :source="problem.inputFormatMd" />
          </template>
          <template v-if="problem.outputFormatMd">
            <h2>输出格式</h2>
            <MarkdownView :source="problem.outputFormatMd" />
          </template>
          <template v-if="problem.samples.length">
            <h2>样例</h2>
            <div v-for="(s, i) in problem.samples" :key="i" class="sample">
              <div>
                <div class="label">
                  输入 {{ i + 1 }}
                  <el-button text size="small" @click="copy(s.input)">复制</el-button>
                </div>
                <pre>{{ s.input }}</pre>
              </div>
              <div>
                <div class="label">
                  输出 {{ i + 1 }}
                  <el-button text size="small" @click="copy(s.output)">复制</el-button>
                </div>
                <pre>{{ s.output }}</pre>
              </div>
            </div>
          </template>
        </el-card>
      </el-col>

      <el-col :xs="24" :md="8">
        <el-card shadow="never">
          <el-tabs>
            <el-tab-pane label="提示">
              <MarkdownView v-if="problem.hintMd" :source="problem.hintMd" />
              <el-empty v-else description="暂无提示" :image-size="60" />
            </el-tab-pane>
            <el-tab-pane v-if="problem.solutionMd !== undefined" label="解题思路">
              <MarkdownView v-if="problem.solutionMd" :source="problem.solutionMd" />
              <el-empty v-else description="暂无解题思路" :image-size="60" />
            </el-tab-pane>
            <el-tab-pane v-if="problem.solutionCode !== undefined" label="解题代码">
              <pre v-if="problem.solutionCode" class="code">{{ problem.solutionCode }}</pre>
              <el-empty v-else description="暂无解题代码" :image-size="60" />
            </el-tab-pane>
            <el-tab-pane label="提交统计">
              <el-descriptions v-if="stats" :column="1" border>
                <el-descriptions-item label="提交数">{{ stats.submitCount }}</el-descriptions-item>
                <el-descriptions-item label="通过数">{{ stats.acceptCount }}</el-descriptions-item>
                <el-descriptions-item label="通过率">{{ rate }}</el-descriptions-item>
              </el-descriptions>
              <el-empty v-else description="暂时无法获取统计" :image-size="60" />
            </el-tab-pane>
            <el-tab-pane label="关联比赛">
              <el-empty description="比赛功能将在 M2 上线" :image-size="60" />
            </el-tab-pane>
          </el-tabs>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<style scoped>
.page {
  min-height: 200px;
}
.banner {
  margin-bottom: 12px;
}
.head {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.head h1 {
  margin: 0;
}
.actions {
  display: flex;
  gap: 8px;
}
.meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  margin: 12px 0 8px;
  color: var(--el-text-color-secondary);
}
.tags {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
h2 {
  font-size: 18px;
  margin: 24px 0 8px;
}
.sample {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-bottom: 12px;
}
.label {
  display: flex;
  justify-content: space-between;
  align-items: center;
  color: var(--el-text-color-secondary);
}
pre {
  margin: 0;
  padding: 8px 12px;
  background: var(--el-fill-color-light);
  border-radius: 4px;
  overflow: auto;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}
.code {
  max-height: 480px;
}
</style>

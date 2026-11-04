<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { ApiError } from '@/api/http'
import { listProblems } from '@/api/problems'
import type { ProblemListItem } from '@/api/types'
import { hasRole } from '@/router/guard'
import { useAuthStore } from '@/stores/auth'
import { DIFFICULTIES, difficultyLabel } from '@/utils/difficulty'
import { parseProblemQuery, toRouteQuery } from '@/utils/problemQuery'

const PAGE_SIZE = 20

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const canManage = computed(() => hasRole(auth.user?.role ?? null, ['problem_admin']))
const query = computed(() => parseProblemQuery(route.query))

const filters = reactive({ keyword: '', tag: '', difficulty: null as number | null })
const showHidden = ref(false)
const items = ref<ProblemListItem[]>([])
const total = ref(0)
const loading = ref(false)
const error = ref('')

// Only the newest request may update the page; slow older ones are ignored.
let latest = 0

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const page = await listProblems({
      ...query.value,
      includeHidden: canManage.value && showHidden.value,
      pageSize: PAGE_SIZE,
    })
    if (seq !== latest) return
    items.value = page.items
    total.value = page.total
  } catch (e) {
    if (seq !== latest) return
    error.value = e instanceof ApiError ? e.message : '加载题目失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

watch(
  () => route.query,
  () => {
    const q = query.value
    filters.keyword = q.keyword
    filters.tag = q.tag
    filters.difficulty = q.difficulty
    void load()
  },
  { immediate: true },
)
watch(showHidden, () => void load())

function go(next: Partial<ReturnType<typeof parseProblemQuery>>) {
  void router.push({ name: 'problems', query: toRouteQuery({ ...query.value, ...next }) })
}

function search() {
  go({
    keyword: filters.keyword.trim(),
    tag: filters.tag.trim(),
    difficulty: filters.difficulty,
    page: 1,
  })
}

function reset() {
  go({ keyword: '', tag: '', difficulty: null, page: 1 })
}

function rate(p: ProblemListItem) {
  return p.submitCount === 0 ? '-' : `${Math.round((p.acceptCount / p.submitCount) * 100)}%`
}
</script>

<template>
  <el-card shadow="never">
    <div class="toolbar">
      <el-form inline @submit.prevent="search">
        <el-form-item>
          <el-input
            v-model="filters.keyword"
            placeholder="搜索标题或标识"
            clearable
            style="width: 220px"
          />
        </el-form-item>
        <el-form-item>
          <el-input v-model="filters.tag" placeholder="标签" clearable style="width: 140px" />
        </el-form-item>
        <el-form-item>
          <el-select v-model="filters.difficulty" placeholder="难度" clearable style="width: 120px">
            <el-option v-for="d in DIFFICULTIES" :key="d.value" :label="d.label" :value="d.value" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" native-type="submit">搜索</el-button>
          <el-button @click="reset">重置</el-button>
        </el-form-item>
      </el-form>
      <div v-if="canManage" class="manage">
        <el-switch v-model="showHidden" active-text="显示未公开" />
        <RouterLink :to="{ name: 'problem-create' }">
          <el-button type="primary">新建题目</el-button>
        </RouterLink>
      </div>
    </div>

    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" class="alert" />

    <el-table v-loading="loading" :data="items" empty-text="没有找到符合条件的题目">
      <el-table-column prop="id" label="编号" width="80" />
      <el-table-column label="题目" min-width="160">
        <template #default="{ row }">
          <RouterLink :to="{ name: 'problem-detail', params: { id: row.id } }">{{
            row.title
          }}</RouterLink>
          <el-tag v-if="!row.isPublic" type="warning" size="small" class="gap">未公开</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="难度" width="90">
        <template #default="{ row }">{{ difficultyLabel(row.difficulty) }}</template>
      </el-table-column>
      <el-table-column label="标签" min-width="120">
        <template #default="{ row }">
          <el-tag
            v-for="t in row.tags"
            :key="t"
            size="small"
            class="gap"
            @click="go({ tag: t, page: 1 })"
            >{{ t }}</el-tag
          >
        </template>
      </el-table-column>
      <el-table-column label="通过 / 提交" width="150">
        <template #default="{ row }"
          >{{ row.acceptCount }} / {{ row.submitCount }}（{{ rate(row) }}）</template
        >
      </el-table-column>
    </el-table>

    <el-pagination
      v-if="total > PAGE_SIZE"
      class="pager"
      layout="prev, pager, next, total"
      :page-size="PAGE_SIZE"
      :current-page="query.page"
      :total="total"
      @current-change="(p: number) => go({ page: p })"
    />
  </el-card>
</template>

<style scoped>
.toolbar {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  align-items: flex-start;
  gap: 8px;
}
.manage {
  display: flex;
  align-items: center;
  gap: 16px;
}
.alert {
  margin-bottom: 12px;
}
.gap {
  margin-left: 6px;
  cursor: pointer;
}
.pager {
  margin-top: 16px;
  justify-content: flex-end;
}
</style>

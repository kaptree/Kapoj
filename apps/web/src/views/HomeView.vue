<script setup lang="ts">
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
</script>

<template>
  <div class="home">
    <el-card shadow="never">
      <h1>欢迎来到 kapOJ</h1>
      <p>
        题库与比赛已经上线，排行榜与 Rating 将在 M4
        推出。完成成就与任务可以赚取金币，再用金币购买并装扮徽章和称号。
      </p>
    </el-card>
    <el-card shadow="never" class="growth">
      <template #header>成长中心</template>
      <div class="links">
        <RouterLink to="/achievements"><el-button>成就墙</el-button></RouterLink>
        <RouterLink to="/tasks"><el-button>任务大厅</el-button></RouterLink>
        <RouterLink to="/shop/badges"><el-button>徽章商城</el-button></RouterLink>
        <RouterLink to="/shop/titles"><el-button>称号商店</el-button></RouterLink>
        <RouterLink v-if="auth.isLoggedIn" to="/appearance">
          <el-button type="primary">装扮管理</el-button>
        </RouterLink>
      </div>
    </el-card>
  </div>
</template>

<style scoped>
.home {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 960px;
}
h1 {
  margin-top: 0;
}
.links {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
</style>

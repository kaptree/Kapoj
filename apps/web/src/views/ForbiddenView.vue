<template>
  <el-result icon="error" title="没有权限" sub-title="你的账号无法访问这个页面。">
    <template #extra>
      <RouterLink to="/"><el-button type="primary">返回首页</el-button></RouterLink>
    </template>
  </el-result>
</template>

<template>
  <el-result icon="warning" title="页面不存在" sub-title="请检查地址是否正确。">
    <template #extra>
      <RouterLink to="/"><el-button type="primary">返回首页</el-button></RouterLink>
    </template>
  </el-result>
</template>

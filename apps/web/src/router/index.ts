import { createRouter, createWebHistory } from 'vue-router'

import AdminLayout from '@/layouts/AdminLayout.vue'
import DefaultLayout from '@/layouts/DefaultLayout.vue'
import { ADMIN_ACCESS_PERMISSIONS, GAMIFICATION_MANAGE_PERMISSIONS, PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'
import { authGuard } from './guard'

declare module 'vue-router' {
  interface RouteMeta {
    requiresAuth?: boolean
    permission?: string
    anyPermission?: string[]
    guestOnly?: boolean
    title?: string
  }
}

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      component: DefaultLayout,
      children: [
        { path: '', name: 'home', component: () => import('@/views/HomeView.vue') },
        {
          path: 'login',
          name: 'login',
          component: () => import('@/views/LoginView.vue'),
          meta: { guestOnly: true, title: '登录' },
        },
        {
          path: 'register',
          name: 'register',
          component: () => import('@/views/RegisterView.vue'),
          meta: { guestOnly: true, title: '注册' },
        },
        {
          path: 'settings/profile',
          name: 'profile-edit',
          component: () => import('@/views/ProfileEditView.vue'),
          meta: { requiresAuth: true, title: '编辑资料' },
        },
        {
          path: 'users/:id',
          name: 'user-profile',
          component: () => import('@/views/UserProfileView.vue'),
          meta: { title: '用户主页' },
        },
        {
          path: 'problems',
          name: 'problems',
          component: () => import('@/views/ProblemListView.vue'),
          meta: { title: '题库' },
        },
        {
          path: 'problems/new',
          name: 'problem-create',
          component: () => import('@/views/ProblemEditView.vue'),
          meta: { requiresAuth: true, permission: PERM.problem.create, title: '新建题目' },
        },
        {
          path: 'problems/:id(\\d+)',
          name: 'problem-detail',
          component: () => import('@/views/ProblemDetailView.vue'),
          meta: { title: '题目' },
        },
        {
          path: 'problems/:id(\\d+)/submit',
          name: 'problem-submit',
          component: () => import('@/views/SubmitView.vue'),
          meta: { requiresAuth: true, title: '提交代码' },
        },
        {
          path: 'submissions',
          name: 'submissions',
          component: () => import('@/views/SubmissionsView.vue'),
          meta: { title: '测评记录' },
        },
        {
          path: 'contests',
          name: 'contests',
          component: () => import('@/views/ContestListView.vue'),
          meta: { title: '比赛' },
        },
        {
          path: 'contests/new',
          name: 'contest-create',
          component: () => import('@/views/ContestEditView.vue'),
          meta: { requiresAuth: true, permission: PERM.contest.create, title: '新建比赛' },
        },
        {
          path: 'contests/:id(\\d+)',
          name: 'contest-detail',
          component: () => import('@/views/ContestDetailView.vue'),
          meta: { title: '比赛详情' },
        },
        {
          path: 'contests/:id(\\d+)/edit',
          name: 'contest-edit',
          component: () => import('@/views/ContestEditView.vue'),
          meta: { requiresAuth: true, permission: PERM.contest.update, title: '编辑比赛' },
        },
        {
          path: 'problems/:id(\\d+)/edit',
          name: 'problem-edit',
          component: () => import('@/views/ProblemEditView.vue'),
          meta: { requiresAuth: true, permission: PERM.problem.update, title: '编辑题目' },
        },
        {
          path: 'leaderboard',
          name: 'leaderboard',
          component: () => import('@/views/LeaderboardView.vue'),
          meta: { title: '排行榜' },
        },
        {
          path: 'achievements',
          name: 'achievements',
          component: () => import('@/views/AchievementsView.vue'),
          meta: { title: '成就' },
        },
        {
          path: 'tasks',
          name: 'tasks',
          component: () => import('@/views/TasksView.vue'),
          meta: { title: '任务' },
        },
        {
          path: 'shop/badges',
          name: 'badge-shop',
          component: () => import('@/views/BadgeShopView.vue'),
          meta: { title: '徽章商城' },
        },
        {
          path: 'shop/titles',
          name: 'title-shop',
          component: () => import('@/views/TitleShopView.vue'),
          meta: { title: '称号商店' },
        },
        {
          path: 'appearance',
          name: 'appearance',
          component: () => import('@/views/AppearanceView.vue'),
          meta: { requiresAuth: true, title: '装扮管理' },
        },
        {
          path: 'forbidden',
          name: 'forbidden',
          component: () => import('@/views/ForbiddenView.vue'),
          meta: { title: '没有权限' },
        },
        {
          path: ':pathMatch(.*)*',
          name: 'not-found',
          component: () => import('@/views/NotFoundView.vue'),
        },
      ],
    },
    {
      path: '/admin',
      component: AdminLayout,
      meta: {
        requiresAuth: true,
        anyPermission: ADMIN_ACCESS_PERMISSIONS,
        title: '管理后台',
      },
      children: [
        {
          path: '',
          name: 'admin-dashboard',
          component: () => import('@/views/AdminDashboardView.vue'),
          meta: { title: '概览' },
        },
        {
          path: 'gamification',
          name: 'gamification-admin',
          component: () => import('@/views/GamificationAdminView.vue'),
          meta: {
            requiresAuth: true,
            anyPermission: GAMIFICATION_MANAGE_PERMISSIONS,
            title: '游戏化管理',
          },
        },
      ],
    },
  ],
})

router.beforeEach((to) => {
  const auth = useAuthStore()
  return authGuard(to, {
    isLoggedIn: auth.isLoggedIn,
    permissions: auth.user?.permissions ?? [],
    isSuperAdmin: auth.isSuperAdmin,
  })
})

router.afterEach((to) => {
  document.title = to.meta.title ? `${to.meta.title} - kapOJ` : 'kapOJ'
})

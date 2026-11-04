import { createRouter, createWebHistory } from 'vue-router'

import DefaultLayout from '@/layouts/DefaultLayout.vue'
import type { Role } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { authGuard } from './guard'

declare module 'vue-router' {
  interface RouteMeta {
    requiresAuth?: boolean
    roles?: Role[]
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
          meta: { requiresAuth: true, roles: ['problem_admin'], title: '新建题目' },
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
          meta: { requiresAuth: true, roles: ['contest_admin'], title: '新建比赛' },
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
          meta: { requiresAuth: true, roles: ['contest_admin'], title: '编辑比赛' },
        },
        {
          path: 'problems/:id(\\d+)/edit',
          name: 'problem-edit',
          component: () => import('@/views/ProblemEditView.vue'),
          meta: { requiresAuth: true, roles: ['problem_admin'], title: '编辑题目' },
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
  ],
})

router.beforeEach((to) => {
  const auth = useAuthStore()
  return authGuard(to, { isLoggedIn: auth.isLoggedIn, role: auth.user?.role ?? null })
})

router.afterEach((to) => {
  document.title = to.meta.title ? `${to.meta.title} - kapOJ` : 'kapOJ'
})

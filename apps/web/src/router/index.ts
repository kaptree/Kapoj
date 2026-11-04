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

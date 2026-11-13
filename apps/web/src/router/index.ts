import { createRouter, createWebHistory } from 'vue-router'

import DefaultLayout from '@/layouts/DefaultLayout.vue'
import {
  ADMIN_ACCESS_PERMISSIONS,
  GAMIFICATION_MANAGE_PERMISSIONS,
  GUEST_PERM,
  PERM,
} from '@/permissions'
import { useAuthStore } from '@/stores/auth'
import { useGuestStore } from '@/stores/guest'
import { authGuard } from './guard'

declare module 'vue-router' {
  interface RouteMeta {
    requiresAuth?: boolean
    permission?: string
    anyPermission?: string[]
    guestOnly?: boolean
    guestPermission?: string
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
          path: 'verify-email',
          name: 'verify-email',
          component: () => import('@/views/VerifyEmailView.vue'),
          meta: { title: '验证邮箱' },
        },
        {
          path: 'forgot-password',
          name: 'forgot-password',
          component: () => import('@/views/ForgotPasswordView.vue'),
          meta: { guestOnly: true, title: '找回密码' },
        },
        {
          path: 'reset-password',
          name: 'reset-password',
          component: () => import('@/views/ResetPasswordView.vue'),
          meta: { guestOnly: true, title: '重置密码' },
        },
        {
          path: 'settings/profile',
          name: 'profile-edit',
          component: () => import('@/views/ProfileEditView.vue'),
          meta: { requiresAuth: true, title: '编辑资料' },
        },
        {
          path: 'settings/security',
          name: 'security-settings',
          component: () => import('@/views/SecuritySettingsView.vue'),
          meta: { requiresAuth: true, title: '账号安全' },
        },
        {
          path: 'settings/ai',
          name: 'ai-settings',
          component: () => import('@/views/AiSettingsView.vue'),
          meta: { requiresAuth: true, title: 'AI 设置' },
        },
        {
          path: 'settings/certificates',
          name: 'certificate-settings',
          component: () => import('@/views/CertificateSettingsView.vue'),
          meta: { requiresAuth: true, title: '我的证书' },
        },
        {
          path: 'verify',
          name: 'certificate-verify',
          component: () => import('@/views/CertificateVerifyView.vue'),
          meta: { title: '证书核验' },
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
          path: 'problems/import',
          name: 'problem-import',
          component: () => import('@/views/ProblemImportView.vue'),
          meta: { requiresAuth: true, permission: PERM.problem.create, title: '导入题包' },
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
          path: 'problems/:id(\\d+)/official-solution',
          name: 'problem-official-solution',
          component: () => import('@/views/ProblemOfficialSolutionView.vue'),
          meta: { title: '官方题解' },
        },
        {
          path: 'problems/:id(\\d+)/solutions',
          name: 'problem-solutions',
          component: () => import('@/views/ProblemSolutionsView.vue'),
          meta: { title: '解题思路' },
        },
        {
          path: 'problems/:id(\\d+)/solutions/:postId(\\d+)',
          name: 'problem-solution-detail',
          component: () => import('@/views/ProblemSolutionDetailView.vue'),
          meta: { title: '思路详情' },
        },
        {
          path: 'problems/:id(\\d+)/statistics',
          name: 'problem-statistics',
          component: () => import('@/views/ProblemStatisticsView.vue'),
          meta: { title: '提交统计' },
        },
        {
          path: 'problems/:id(\\d+)/contests',
          name: 'problem-contests',
          component: () => import('@/views/ProblemContestsView.vue'),
          meta: { title: '关联比赛' },
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
          path: 'learning-paths',
          name: 'learning-paths',
          component: () => import('@/views/LearningPathListView.vue'),
          meta: { title: '学习路线' },
        },
        {
          path: 'learning-paths/new',
          name: 'learning-path-create',
          component: () => import('@/views/LearningPathEditView.vue'),
          meta: { requiresAuth: true, title: '新建学习路线' },
        },
        {
          path: 'learning-paths/:id(\\d+)',
          name: 'learning-path-detail',
          component: () => import('@/views/LearningPathDetailView.vue'),
          meta: { title: '学习路线' },
        },
        {
          path: 'learning-paths/:id(\\d+)/edit',
          name: 'learning-path-edit',
          component: () => import('@/views/LearningPathEditView.vue'),
          meta: { requiresAuth: true, title: '编辑学习路线' },
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
          path: 'problems/:id(\\d+)/testcase-generator',
          name: 'problem-testcase-generator',
          component: () => import('@/views/TestcaseGeneratorView.vue'),
          meta: {
            requiresAuth: true,
            permission: PERM.problem.testcase.manage,
            title: 'AI 造数据',
          },
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
          meta: { guestPermission: GUEST_PERM.growth, title: '成就' },
        },
        {
          path: 'tasks',
          name: 'tasks',
          component: () => import('@/views/TasksView.vue'),
          meta: { guestPermission: GUEST_PERM.growth, title: '任务' },
        },
        {
          path: 'shop/badges',
          name: 'badge-shop',
          component: () => import('@/views/BadgeShopView.vue'),
          meta: { guestPermission: GUEST_PERM.growth, title: '徽章商城' },
        },
        {
          path: 'shop/titles',
          name: 'title-shop',
          component: () => import('@/views/TitleShopView.vue'),
          meta: { guestPermission: GUEST_PERM.growth, title: '称号商店' },
        },
        {
          path: 'appearance',
          name: 'appearance',
          component: () => import('@/views/AppearanceView.vue'),
          meta: { requiresAuth: true, title: '装扮管理' },
        },
        {
          path: 'notifications',
          name: 'notifications',
          component: () => import('@/views/NotificationsView.vue'),
          meta: { requiresAuth: true, title: '通知' },
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
          meta: { title: '页面不存在' },
        },
      ],
    },
    {
      path: '/admin',
      component: () => import('@/layouts/AdminLayout.vue'),
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
          path: 'users',
          name: 'admin-users',
          component: () => import('@/views/AdminUsersView.vue'),
          meta: { requiresAuth: true, permission: PERM.user.read, title: '用户管理' },
        },
        {
          path: 'roles',
          name: 'admin-roles',
          component: () => import('@/views/AdminRolesView.vue'),
          meta: { requiresAuth: true, permission: PERM.role.read, title: '角色权限' },
        },
        {
          path: 'audit',
          name: 'admin-audit',
          component: () => import('@/views/AdminAuditView.vue'),
          meta: { requiresAuth: true, permission: PERM.admin.audit.view, title: '审计日志' },
        },
        {
          path: 'rating',
          name: 'admin-rating',
          component: () => import('@/views/AdminRatingView.vue'),
          meta: {
            requiresAuth: true,
            permission: PERM.rating.settings.manage,
            title: 'Rating 设置',
          },
        },
        {
          path: 'ai',
          name: 'admin-ai',
          component: () => import('@/views/AdminAiView.vue'),
          meta: {
            requiresAuth: true,
            permission: PERM.ai.settings.manage,
            title: 'AI 设置',
          },
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
        {
          path: 'learning-paths',
          name: 'admin-learning-paths',
          component: () => import('@/views/AdminLearningPathsView.vue'),
          meta: {
            requiresAuth: true,
            anyPermission: [PERM.learningPath.official.manage, PERM.learningPath.admin.read],
            title: '学习路线管理',
          },
        },
        {
          path: 'certificates',
          name: 'admin-certificates',
          component: () => import('@/views/AdminCertificatesView.vue'),
          meta: {
            requiresAuth: true,
            permission: PERM.certificate.manage,
            title: '证书审核',
          },
        },
        {
          path: 'site',
          name: 'admin-site',
          component: () => import('@/views/AdminSiteView.vue'),
          meta: {
            requiresAuth: true,
            permission: PERM.site.settings.manage,
            title: '站点设置',
          },
        },
        {
          path: 'mail',
          name: 'admin-mail',
          component: () => import('@/views/AdminMailView.vue'),
          meta: {
            requiresAuth: true,
            permission: PERM.mail.settings.manage,
            title: '邮件设置',
          },
        },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  const guest = useGuestStore()
  if (!auth.isLoggedIn) await guest.ensureLoaded()
  return authGuard(to, {
    isLoggedIn: auth.isLoggedIn,
    permissions: auth.user?.permissions ?? [],
    isSuperAdmin: auth.isSuperAdmin,
    guestPermissions: guest.permissions,
  })
})

router.afterEach((to) => {
  document.title = to.meta.title ? `${to.meta.title} - kapOJ` : 'kapOJ'
})

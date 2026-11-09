package rbac

// PermissionCode 是稳定的权限码，格式为 domain.action。
type PermissionCode string

const (
	PermAdminDashboardView   PermissionCode = "admin.dashboard.view"
	PermAdminAuditView       PermissionCode = "admin.audit.view"
	PermSystemSettingsManage PermissionCode = "system.settings.manage"

	PermUserRead              PermissionCode = "user.read"
	PermUserUpdate            PermissionCode = "user.update"
	PermUserBan               PermissionCode = "user.ban"
	PermUserCoinsAdjust       PermissionCode = "user.coins.adjust"
	PermUserRolesAssign       PermissionCode = "user.roles.assign"
	PermRoleRead              PermissionCode = "role.read"
	PermRoleCreate            PermissionCode = "role.create"
	PermRoleUpdate            PermissionCode = "role.update"
	PermRoleDelete            PermissionCode = "role.delete"
	PermRolePermissionsUpdate PermissionCode = "role.permissions.update"

	PermProblemViewHidden         PermissionCode = "problem.view_hidden"
	PermProblemCreate             PermissionCode = "problem.create"
	PermProblemUpdate             PermissionCode = "problem.update"
	PermProblemDelete             PermissionCode = "problem.delete"
	PermProblemPublish            PermissionCode = "problem.publish"
	PermProblemTestcaseManage     PermissionCode = "problem.testcase.manage"
	PermProblemCheckerManage      PermissionCode = "problem.checker.manage"
	PermSubmissionViewLiveContest PermissionCode = "submission.view_live_contest"

	PermContestViewHidden         PermissionCode = "contest.view_hidden"
	PermContestCreate             PermissionCode = "contest.create"
	PermContestUpdate             PermissionCode = "contest.update"
	PermContestDelete             PermissionCode = "contest.delete"
	PermContestParticipantRead    PermissionCode = "contest.participant.read"
	PermContestAnnouncementManage PermissionCode = "contest.announcement.manage"

	PermGamificationAchievementManage PermissionCode = "gamification.achievement.manage"
	PermGamificationTaskManage        PermissionCode = "gamification.task.manage"
	PermGamificationBadgeManage       PermissionCode = "gamification.badge.manage"
	PermGamificationTitleManage       PermissionCode = "gamification.title.manage"
	PermGamificationConfigManage      PermissionCode = "gamification.config.manage"

	PermLearningPathOfficialManage PermissionCode = "learning_path.official.manage"
	PermLearningPathAdminRead      PermissionCode = "learning_path.admin.read"

	PermRatingSettingsManage PermissionCode = "rating.settings.manage"
	PermAISettingsManage     PermissionCode = "ai.settings.manage"

	PermCertificateManage PermissionCode = "certificate.manage"

	PermSiteSettingsManage PermissionCode = "site.settings.manage"
)

const (
	PermissionGroupAdmin        = "管理后台"
	PermissionGroupUser         = "用户与角色"
	PermissionGroupProblem      = "题库"
	PermissionGroupSubmission   = "测评记录"
	PermissionGroupContest      = "比赛"
	PermissionGroupGamification = "游戏化"
	PermissionGroupLearning     = "学习路线"
	PermissionGroupRating       = "Rating"
	PermissionGroupAI           = "AI"
	PermissionGroupCertificate  = "证书"
	PermissionGroupSite         = "站点"
)

// PermissionDef 描述权限目录中的一项，用于首次 Seed 与权限矩阵展示。
type PermissionDef struct {
	Code        PermissionCode
	Name        string
	GroupName   string
	Description string
	SortOrder   int
}

var permissionCatalog = []PermissionDef{
	{Code: PermAdminDashboardView, Name: "访问管理仪表盘", GroupName: PermissionGroupAdmin, Description: "访问管理后台仪表盘与统计数据"},
	{Code: PermAdminAuditView, Name: "查看审计日志", GroupName: PermissionGroupAdmin, Description: "查看管理员操作审计日志"},
	{Code: PermSystemSettingsManage, Name: "管理系统设置", GroupName: PermissionGroupAdmin, Description: "修改平台系统配置（预留）"},

	{Code: PermUserRead, Name: "查看用户", GroupName: PermissionGroupUser, Description: "查看用户列表与用户详情"},
	{Code: PermUserUpdate, Name: "编辑用户资料", GroupName: PermissionGroupUser, Description: "编辑用户昵称、简介与头像链接"},
	{Code: PermUserBan, Name: "封禁与解封用户", GroupName: PermissionGroupUser, Description: "封禁或解封用户账号"},
	{Code: PermUserCoinsAdjust, Name: "调整用户金币", GroupName: PermissionGroupUser, Description: "增加或扣减用户金币"},
	{Code: PermUserRolesAssign, Name: "分配用户角色", GroupName: PermissionGroupUser, Description: "为用户分配或移除角色"},
	{Code: PermRoleRead, Name: "查看角色与权限", GroupName: PermissionGroupUser, Description: "查看角色列表与权限目录"},
	{Code: PermRoleCreate, Name: "创建角色", GroupName: PermissionGroupUser, Description: "创建自定义角色"},
	{Code: PermRoleUpdate, Name: "编辑角色", GroupName: PermissionGroupUser, Description: "编辑自定义角色的名称与描述"},
	{Code: PermRoleDelete, Name: "删除角色", GroupName: PermissionGroupUser, Description: "删除自定义角色"},
	{Code: PermRolePermissionsUpdate, Name: "修改角色权限", GroupName: PermissionGroupUser, Description: "调整角色拥有的权限集合"},

	{Code: PermProblemViewHidden, Name: "查看未公开题目", GroupName: PermissionGroupProblem, Description: "查看未公开题目及其提交记录"},
	{Code: PermProblemCreate, Name: "创建题目", GroupName: PermissionGroupProblem, Description: "创建新题目"},
	{Code: PermProblemUpdate, Name: "编辑题目", GroupName: PermissionGroupProblem, Description: "编辑题面与题目元数据"},
	{Code: PermProblemDelete, Name: "删除题目", GroupName: PermissionGroupProblem, Description: "删除题目或将其下线"},
	{Code: PermProblemPublish, Name: "发布与下线题目", GroupName: PermissionGroupProblem, Description: "公开或下线题目"},
	{Code: PermProblemTestcaseManage, Name: "管理测试数据", GroupName: PermissionGroupProblem, Description: "上传、编辑、排序或删除测试数据"},
	{Code: PermProblemCheckerManage, Name: "管理检查器", GroupName: PermissionGroupProblem, Description: "上传题目检查器"},

	{Code: PermSubmissionViewLiveContest, Name: "查看赛时提交", GroupName: PermissionGroupSubmission, Description: "比赛进行中查看他人代码、成绩与测试点"},

	{Code: PermContestViewHidden, Name: "查看非公开比赛", GroupName: PermissionGroupContest, Description: "查看非公开比赛及其题目与榜单"},
	{Code: PermContestCreate, Name: "创建比赛", GroupName: PermissionGroupContest, Description: "创建新比赛"},
	{Code: PermContestUpdate, Name: "编辑比赛", GroupName: PermissionGroupContest, Description: "编辑比赛信息"},
	{Code: PermContestDelete, Name: "删除比赛", GroupName: PermissionGroupContest, Description: "删除比赛"},
	{Code: PermContestParticipantRead, Name: "查看报名用户", GroupName: PermissionGroupContest, Description: "查看比赛报名用户列表"},
	{Code: PermContestAnnouncementManage, Name: "管理比赛公告", GroupName: PermissionGroupContest, Description: "创建、编辑与删除比赛公告"},

	{Code: PermGamificationAchievementManage, Name: "管理成就", GroupName: PermissionGroupGamification, Description: "创建、编辑与删除成就"},
	{Code: PermGamificationTaskManage, Name: "管理任务", GroupName: PermissionGroupGamification, Description: "创建、编辑与删除任务"},
	{Code: PermGamificationBadgeManage, Name: "管理徽章", GroupName: PermissionGroupGamification, Description: "创建、编辑与删除徽章"},
	{Code: PermGamificationTitleManage, Name: "管理称号", GroupName: PermissionGroupGamification, Description: "创建、编辑与删除称号"},
	{Code: PermGamificationConfigManage, Name: "管理游戏化配置", GroupName: PermissionGroupGamification, Description: "调整装备上限等游戏化系统配置"},

	{Code: PermLearningPathOfficialManage, Name: "管理官方学习路线", GroupName: PermissionGroupLearning, Description: "创建、编辑与上下线官方学习路线"},
	{Code: PermLearningPathAdminRead, Name: "查看全部学习路线", GroupName: PermissionGroupLearning, Description: "查看所有学习路线及已下架路线"},

	{Code: PermRatingSettingsManage, Name: "管理 Rating 设置", GroupName: PermissionGroupRating, Description: "配置 Rating 衰减与赛季重置策略"},

	{Code: PermAISettingsManage, Name: "管理 AI 设置", GroupName: PermissionGroupAI, Description: "启用或关闭 AI 功能"},

	{Code: PermCertificateManage, Name: "管理证书", GroupName: PermissionGroupCertificate, Description: "审核证书认证并维护证书类型"},

	{Code: PermSiteSettingsManage, Name: "管理站点设置", GroupName: PermissionGroupSite, Description: "编辑首页内容、页脚、Logo 与站点图标"},
}

// PermissionCodes 是按目录顺序排列的全部权限码。
var PermissionCodes = func() []PermissionCode {
	codes := make([]PermissionCode, len(permissionCatalog))
	for i, def := range permissionCatalog {
		codes[i] = def.Code
	}
	return codes
}()

const (
	RoleCodeSuperAdmin        = "super_admin"
	RoleCodeContentAdmin      = "content_admin"
	RoleCodeContestAdmin      = "contest_admin"
	RoleCodeGamificationAdmin = "gamification_admin"
	RoleCodeUserAdmin         = "user_admin"
	RoleCodeSecurityAdmin     = "security_admin"
	RoleCodeAuditor           = "auditor"
)

// RoleDef 描述一个内置角色及其初始权限集合。
type RoleDef struct {
	Code        string
	Name        string
	Description string
	Permissions []PermissionCode
}

var builtinRoleDefs = []RoleDef{
	{
		Code: RoleCodeSuperAdmin, Name: "超级管理员",
		Description: "绕过所有权限检查；不可删除或修改权限",
	},
	{
		Code: RoleCodeContentAdmin, Name: "内容管理员",
		Description: "管理题库与官方学习路线",
		Permissions: []PermissionCode{
			PermAdminDashboardView,
			PermProblemViewHidden, PermProblemCreate, PermProblemUpdate, PermProblemDelete,
			PermProblemPublish, PermProblemTestcaseManage, PermProblemCheckerManage,
			PermLearningPathOfficialManage, PermLearningPathAdminRead,
		},
	},
	{
		Code: RoleCodeContestAdmin, Name: "比赛管理员",
		Description: "管理比赛并查看赛时提交",
		Permissions: []PermissionCode{
			PermAdminDashboardView,
			PermContestViewHidden, PermContestCreate, PermContestUpdate, PermContestDelete,
			PermContestParticipantRead, PermContestAnnouncementManage,
			PermSubmissionViewLiveContest,
		},
	},
	{
		Code: RoleCodeGamificationAdmin, Name: "游戏化管理员",
		Description: "管理成就、任务、徽章、称号与游戏化配置",
		Permissions: []PermissionCode{
			PermAdminDashboardView,
			PermGamificationAchievementManage, PermGamificationTaskManage,
			PermGamificationBadgeManage, PermGamificationTitleManage,
			PermGamificationConfigManage,
		},
	},
	{
		Code: RoleCodeUserAdmin, Name: "用户管理员",
		Description: "查看和编辑用户、封禁与金币调整",
		Permissions: []PermissionCode{
			PermAdminDashboardView,
			PermUserRead, PermUserUpdate, PermUserBan, PermUserCoinsAdjust,
			PermRoleRead,
		},
	},
	{
		Code: RoleCodeSecurityAdmin, Name: "安全管理员",
		Description: "管理角色与权限分配，查看审计日志",
		Permissions: []PermissionCode{
			PermAdminDashboardView, PermAdminAuditView, PermUserRead,
			PermRoleRead, PermRoleCreate, PermRoleUpdate, PermRoleDelete,
			PermRolePermissionsUpdate, PermUserRolesAssign,
		},
	},
	{
		Code: RoleCodeAuditor, Name: "审计员",
		Description: "只读查看审计日志、用户与角色",
		Permissions: []PermissionCode{
			PermAdminDashboardView, PermAdminAuditView, PermUserRead, PermRoleRead,
		},
	},
}

// AllPermissions 返回权限目录的副本，调用方修改不会影响内置目录。
func AllPermissions() []PermissionDef {
	defs := make([]PermissionDef, len(permissionCatalog))
	copy(defs, permissionCatalog)
	for i := range defs {
		if defs[i].SortOrder == 0 {
			defs[i].SortOrder = (i + 1) * 10
		}
	}
	return defs
}

// BuiltinRoles 返回内置角色定义的副本，调用方修改不会影响内置角色。
func BuiltinRoles() []RoleDef {
	defs := make([]RoleDef, len(builtinRoleDefs))
	for i, def := range builtinRoleDefs {
		defs[i] = def
		defs[i].Permissions = append([]PermissionCode(nil), def.Permissions...)
	}
	return defs
}

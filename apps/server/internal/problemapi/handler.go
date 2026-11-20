package problemapi

import (
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/difficulty"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/problemimport"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/social"
	"github.com/kaptree/kapoj/apps/server/internal/testcasegen"
)

const (
	defaultTestcaseScore = 10
	// Multipart parts above this size are spooled to temporary files rather
	// than kept in memory.
	multipartMemory = 1 << 20
)

// RegisterRoutes mounts the problem endpoints. api, problemJSON and uploads
// must already run middleware.Authenticate. problemJSON carries a larger
// limit because a statement plus samples can exceed the default 1 MiB;
// uploads carries the limit used by the file upload endpoints.
// ImportLimitProvider exposes the current configurable import limits.
type ImportLimitProvider interface {
	Get(ctx context.Context) (*problemimport.SettingsView, error)
}

type AcceptedChecker interface {
	HasAccepted(ctx context.Context, userID, problemID int64) (bool, error)
}

type ContestAccess interface {
	CanViewProblem(ctx context.Context, userID, problemID int64, now time.Time) (bool, error)
	IsProblemLocked(ctx context.Context, problemID int64, now time.Time) (bool, error)
}

// IdentityLookup 批量读取用户公开身份；由 gamification.Service 实现。
type IdentityLookup interface {
	Identities(ctx context.Context, userIDs []int64) (map[int64]gamification.Identity, error)
}

// Options 配置题目接口的可选依赖。
type Options struct {
	RBAC        *rbac.Service
	Solutions   *problem.OfficialSolutionService
	Discussions *problem.DiscussionService
	// Social 是社交服务，供点赞/回复/删除钩子使用（Task 2 接入）。
	Social *social.Service
	// Difficulty 提供难度设置（默认分、AI 开关与超时）。为 nil 时创建接口
	// 保持旧行为（直接采用请求分值），便于分阶段接入。
	Difficulty *difficulty.SettingsService
	// AI 是平台 AI 难度评估器；AISettings 是 AI 总开关。
	AI         DifficultyAssessor
	AISettings AISettings
	// MaxImportFileBytes overrides the uploaded HydroOJ package file limit.
	// Zero means problem.MaxHydroZipBytes. Tests may set a small value.
	MaxImportFileBytes int64
	// MaxImportUncompressedBytes overrides the total decompressed bytes
	// allowed during a HydroOJ import. Zero means the package default
	// (problem.MaxHydroUncompressed). Tests may set a small value.
	MaxImportUncompressedBytes int64
	// ImportSettings provides the current admin-configured package size limit.
	// When nil, MaxImportFileBytes / package defaults are used.
	ImportSettings ImportLimitProvider
	// MaxBatchBodyBytes caps the batch testcase multipart route. Zero means
	// the historical 210 MiB cap.
	MaxBatchBodyBytes int64
	// TestcaseGen 是 AI 出题造数据服务；非 nil 时注册相关路由。
	TestcaseGen *testcasegen.Service
	// IdentityLookup 为提交统计明细补充公开徽章/称号。
	IdentityLookup IdentityLookup
}

// auditTargetProblem 是内容管理审计的 target type。
const auditTargetProblem = "problem"

func RegisterRoutes(
	api, problemJSON, uploads, imports gin.IRouter,
	s *problem.Service,
	accepted AcceptedChecker,
	contests ContestAccess,
	opts ...Options,
) {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	maxImport := o.MaxImportFileBytes
	if maxImport <= 0 {
		maxImport = problem.MaxHydroZipBytes
	}
	if o.MaxImportUncompressedBytes > 0 {
		problem.WithHydroUncompressedLimit(o.MaxImportUncompressedBytes)(s)
	}
	h := &handler{svc: s, accepted: accepted, contests: contests, rbac: o.RBAC, solutions: o.Solutions, discussions: o.Discussions, social: o.Social, identities: o.IdentityLookup, maxImportFileBytes: maxImport, maxImportUncompressedBytes: o.MaxImportUncompressedBytes, importSettings: o.ImportSettings, testcaseGen: o.TestcaseGen, difficulty: o.Difficulty, ai: o.AI, aiSettings: o.AISettings}

	// resolver 为 nil 时 RequirePermission 会拒绝所有写操作，LoadAccess 为
	// 空操作；生产环境 router 始终传入 RBAC service。
	var resolver middleware.PermissionResolver
	if h.rbac != nil {
		resolver = h.rbac
	}
	load := middleware.LoadAccess(resolver)
	require := func(code string) gin.HandlerFunc {
		return middleware.RequirePermission(resolver, code)
	}
	audit := func(code string) gin.HandlerFunc {
		return middleware.AuditAdminAction(h.rbac, code, auditTargetProblem)
	}

	api.GET("/problems", load, h.list)
	api.GET("/problems/:id", load, h.get)
	api.GET("/problems/:id/statistics", load, h.statistics)
	api.GET("/problems/:id/submission-stats", load, h.submissionStats)
	api.GET("/problems/:id/attachments", load, h.listAttachments)
	api.GET("/problems/:id/attachments/:aid", load, h.downloadAttachment)
	api.GET("/problems/:id/assets/:aid", load, h.statementImage)
	api.GET("/problems/:id/package/info", middleware.RequireAuth(), load, h.packageInfo)
	api.GET("/problems/:id/package", middleware.RequireAuth(), load, h.downloadPackage)
	api.POST("/problems/bulk/export", require(string(rbac.PermProblemManageAll)), h.exportProblems)
	api.POST("/problems/bulk/actions", require(string(rbac.PermProblemManageAll)), h.bulkActions)
	api.DELETE("/problems/:id/attachments/:aid", require(string(rbac.PermProblemUpdate)), audit(string(rbac.PermProblemUpdate)), h.deleteAttachment)

	api.GET("/problems/:id/official-solution", load, h.getOfficialSolution)
	api.PUT("/problems/:id/official-solution", middleware.RequireAuth(), load, h.updateOfficialSolution)
	api.POST("/problems/:id/official-solution/purchase", middleware.RequireAuth(), load, h.purchaseOfficialSolution)

	api.GET("/problems/:id/solutions", load, h.listPosts)
	api.POST("/problems/:id/solutions", middleware.RequireAuth(), load, h.createPost)
	api.GET("/problems/:id/solutions/:postId", load, h.getPost)
	api.PUT("/problems/:id/solutions/:postId", middleware.RequireAuth(), load, h.updatePost)
	api.DELETE("/problems/:id/solutions/:postId", middleware.RequireAuth(), load, h.deletePost)
	api.POST("/problems/:id/solutions/:postId/replies", middleware.RequireAuth(), load, h.createReply)
	api.PUT("/problems/:id/solutions/:postId/replies/:replyId", middleware.RequireAuth(), load, h.updateReply)
	api.DELETE("/problems/:id/solutions/:postId/replies/:replyId", middleware.RequireAuth(), load, h.deleteReply)
	api.POST("/problems/:id/solutions/:postId/like", middleware.RequireAuth(), load, h.likePost)
	api.DELETE("/problems/:id/solutions/:postId/like", middleware.RequireAuth(), load, h.unlikePost)

	problemJSON.POST("/problems", require(string(rbac.PermProblemCreate)), audit(string(rbac.PermProblemCreate)), h.create)
	problemJSON.PUT("/problems/:id", require(string(rbac.PermProblemUpdate)), audit(string(rbac.PermProblemUpdate)), h.update)
	api.DELETE("/problems/:id", require(string(rbac.PermProblemDelete)), audit(string(rbac.PermProblemDelete)), h.delete)
	api.POST("/problems/:id/difficulty/assess", require(string(rbac.PermProblemUpdate)), h.assessProblemDifficulty)
	api.GET("/problems/:id/difficulty/history", require(string(rbac.PermProblemUpdate)), h.difficultyHistory)

	api.GET("/problems/:id/testcases", require(string(rbac.PermProblemTestcaseManage)), h.listTestcases)
	api.GET("/problems/:id/testcases/:tid/content", require(string(rbac.PermProblemTestcaseManage)), h.getTestcaseContent)
	problemJSON.PUT("/problems/:id/testcases/:tid/content", require(string(rbac.PermProblemTestcaseManage)), audit(string(rbac.PermProblemTestcaseManage)), h.updateTestcaseContent)
	problemJSON.POST("/problems/:id/testcases/online", require(string(rbac.PermProblemTestcaseManage)), audit(string(rbac.PermProblemTestcaseManage)), h.addOnlineTestcase)
	api.PUT("/problems/:id/testcases/:tid", require(string(rbac.PermProblemTestcaseManage)), audit(string(rbac.PermProblemTestcaseManage)), h.updateTestcase)
	api.POST("/problems/:id/testcases/reorder", require(string(rbac.PermProblemTestcaseManage)), audit(string(rbac.PermProblemTestcaseManage)), h.reorderTestcases)
	api.DELETE("/problems/:id/testcases/:tid", require(string(rbac.PermProblemTestcaseManage)), audit(string(rbac.PermProblemTestcaseManage)), h.deleteTestcase)

	uploads.POST("/problems/:id/testcases", require(string(rbac.PermProblemTestcaseManage)), audit(string(rbac.PermProblemTestcaseManage)), h.addTestcase)
	uploads.POST("/problems/:id/checker", require(string(rbac.PermProblemCheckerManage)), audit(string(rbac.PermProblemCheckerManage)), h.uploadChecker)
	uploads.POST("/problems/:id/attachments", require(string(rbac.PermProblemUpdate)), audit(string(rbac.PermProblemUpdate)), h.uploadAttachment)
	uploads.POST("/problems/:id/assets", require(string(rbac.PermProblemUpdate)), audit(string(rbac.PermProblemUpdate)), h.uploadStatementImage)

	if h.testcaseGen != nil {
		api.POST("/problems/:id/testcase-generations", require(string(rbac.PermProblemTestcaseManage)), audit(string(rbac.PermProblemTestcaseManage)), h.createTestcaseGeneration)
		api.GET("/problems/:id/testcase-generations/:genId", require(string(rbac.PermProblemTestcaseManage)), h.getTestcaseGeneration)
		api.GET("/problems/:id/testcase-generations/:genId/cases/:caseId/preview", require(string(rbac.PermProblemTestcaseManage)), h.previewTestcaseGeneration)
		api.POST("/problems/:id/testcase-generations/:genId/confirm", require(string(rbac.PermProblemTestcaseManage)), audit(string(rbac.PermProblemTestcaseManage)), h.confirmTestcaseGeneration)
		api.DELETE("/problems/:id/testcase-generations/:genId", require(string(rbac.PermProblemTestcaseManage)), audit(string(rbac.PermProblemTestcaseManage)), h.discardTestcaseGeneration)
	}

	api.GET("/problems/import/limits", load, require(string(rbac.PermProblemCreate)), h.importLimitsView)
	maxBatchBody := o.MaxBatchBodyBytes
	if maxBatchBody <= 0 {
		maxBatchBody = 210 << 20
	}
	imports.POST("/problems/import/hydro", require(string(rbac.PermProblemCreate)), h.importHydro)
	imports.POST("/problems/import/package", require(string(rbac.PermProblemCreate)), h.importPackage)
	imports.POST("/problems/:id/testcases/batch", middleware.MaxBody(maxBatchBody),
		require(string(rbac.PermProblemTestcaseManage)), audit(string(rbac.PermProblemTestcaseManage)), h.addTestcasesBatch)
}

type handler struct {
	svc         *problem.Service
	accepted    AcceptedChecker
	contests    ContestAccess
	rbac        *rbac.Service
	solutions   *problem.OfficialSolutionService
	discussions *problem.DiscussionService
	social      *social.Service
	identities  IdentityLookup

	maxImportFileBytes         int64
	maxImportUncompressedBytes int64
	importSettings             ImportLimitProvider
	testcaseGen                *testcasegen.Service

	difficulty *difficulty.SettingsService
	ai         DifficultyAssessor
	aiSettings AISettings
}

func viewer(c *gin.Context) problem.Viewer {
	access, ok := middleware.CurrentAccess(c)
	if !ok || access == nil {
		return problem.Viewer{}
	}
	var userID int64
	if claims, ok := middleware.CurrentClaims(c); ok {
		userID = claims.UserID
	}
	canManageAll := access.IsSuper || access.Has(string(rbac.PermProblemManageAll))
	canManageOwn := canManageAll || access.Has(string(rbac.PermProblemCreate)) ||
		access.Has(string(rbac.PermProblemUpdate)) ||
		access.Has(string(rbac.PermProblemDelete)) ||
		access.Has(string(rbac.PermProblemPublish)) ||
		access.Has(string(rbac.PermProblemTestcaseManage)) ||
		access.Has(string(rbac.PermProblemCheckerManage))
	return problem.Viewer{
		UserID:        userID,
		CanViewHidden: access.Has(string(rbac.PermProblemViewHidden)) || canManageAll,
		CanViewLocked: access.Has(string(rbac.PermProblemViewHidden)) ||
			access.Has(string(rbac.PermContestUpdate)) || canManageAll,
		CanViewSolution:    canViewSolution(access) || canManageAll,
		CanManageAll:       canManageAll,
		CanManageOwn:       canManageOwn,
		CanViewLiveContest: access.Has(string(rbac.PermSubmissionViewLiveContest)),
	}
}

// canManageThis reports whether the viewer may manage this specific problem.
func canManageThis(v problem.Viewer, p *problem.Problem) bool {
	return v.CanManageAll || (v.CanManageOwn && p.CreatedBy == v.UserID)
}

// canViewSolution 表示当前用户是否能看到题解字段。只有题面/元数据编辑、删除、
// 发布、测试数据或 checker 管理权限可看题解；仅 view_hidden 的只读角色不能看。
func canViewSolution(access *rbac.Access) bool {
	return access.Has(string(rbac.PermProblemUpdate)) ||
		access.Has(string(rbac.PermProblemDelete)) ||
		access.Has(string(rbac.PermProblemPublish)) ||
		access.Has(string(rbac.PermProblemTestcaseManage)) ||
		access.Has(string(rbac.PermProblemCheckerManage))
}

// currentHas reports whether the current request carries a permission code.
func currentHas(c *gin.Context, code string) bool {
	access, ok := middleware.CurrentAccess(c)
	return ok && access != nil && access.Has(code)
}

// currentCanManageContest reports whether the current user may edit contests.
func currentCanManageContest(c *gin.Context) bool {
	return currentHas(c, string(rbac.PermContestUpdate))
}

// problemForManage loads a problem and verifies the current user may manage it.
// Admins/manage_all may manage any problem; authors may manage only their own.
// Hidden problems owned by someone else look missing to users without hidden
// read access.
func (h *handler) problemForManage(c *gin.Context, id int64) (*problem.Problem, bool) {
	p, err := h.svc.Get(c.Request.Context(), problem.Viewer{CanViewHidden: true}, id)
	if err != nil {
		httpx.Fail(c, err)
		return nil, false
	}
	v := viewer(c)
	if canManageThis(v, p) {
		return p, true
	}
	if !p.IsPublic && !v.CanViewHidden {
		httpx.Fail(c, httpx.ErrNotFound)
	} else {
		httpx.Fail(c, httpx.ErrForbidden)
	}
	return nil, false
}

// currentCanViewLockedStats reports whether the current user may see statistics
// for a problem locked by an unfinished contest.
func currentCanViewLockedStats(c *gin.Context) bool {
	return currentHas(c, string(rbac.PermProblemViewHidden)) ||
		currentHas(c, string(rbac.PermContestUpdate)) || currentIsSuper(c)
}

// lockedProblemAccess resolves whether a problem is locked by an unfinished
// contest and, when it is, whether the current request may still reach it.
// participantAllowed is true for a registered participant of a running contest
// or, when the problem is not locked at all, for everyone.
func (h *handler) lockedProblemAccess(c *gin.Context, problemID int64) (locked, participantAllowed, manager bool, err error) {
	if h.contests == nil {
		return false, true, false, nil
	}
	locked, err = h.contests.IsProblemLocked(c.Request.Context(), problemID, time.Now())
	if err != nil || !locked {
		return locked, !locked, false, err
	}
	manager = viewer(c).CanViewHidden || currentCanManageContest(c) || currentIsSuper(c)
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		return true, false, manager, nil
	}
	participantAllowed, err = h.contests.CanViewProblem(c.Request.Context(), claims.UserID, problemID, time.Now())
	return true, participantAllowed, manager, err
}

// lockedResourceBlocked reports whether a contest-locked problem's solution and
// discussion resources must look missing to the current request.
func (h *handler) lockedResourceBlocked(c *gin.Context, problemID int64) (bool, error) {
	if h.contests == nil {
		return false, nil
	}
	locked, err := h.contests.IsProblemLocked(c.Request.Context(), problemID, time.Now())
	if err != nil || !locked {
		return false, err
	}
	return !viewer(c).CanViewSolution && !currentIsSuper(c), nil
}

// pathID reads a positive integer path parameter; anything else is reported
// as "not found" so malformed ids reveal nothing.
func pathID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrNotFound)
		return 0, false
	}
	return id, true
}

func (h *handler) list(c *gin.Context) {
	v := viewer(c)
	q := problem.ListQuery{
		Keyword:       c.Query("keyword"),
		Tag:           c.Query("tag"),
		IncludeHidden: c.Query("includeHidden") == "true",
	}
	scope := c.Query("scope")
	switch scope {
	case "", "public":
		if scope == "public" {
			q.IncludeHidden = false
		}
	case "mine":
		claims, ok := middleware.CurrentClaims(c)
		if !ok || claims.UserID <= 0 {
			httpx.Fail(c, httpx.ErrUnauthorized)
			return
		}
		if !v.CanManageOwn && !v.CanManageAll {
			httpx.Fail(c, httpx.ErrForbidden)
			return
		}
		q.OwnerID = claims.UserID
		// The management view defaults to showing the author's hidden drafts.
		if c.Query("includeHidden") == "" {
			q.IncludeHidden = true
		}
	case "all":
		if !v.CanManageAll {
			httpx.Fail(c, httpx.ErrForbidden)
			return
		}
		q.IncludeHidden = true
	default:
		httpx.Fail(c, httpx.ErrValidation("scope 必须是 public、mine 或 all"))
		return
	}
	if d := c.Query("difficulty"); d != "" {
		n, err := strconv.Atoi(d)
		if err != nil {
			httpx.Fail(c, httpx.ErrValidation("difficulty 必须是整数"))
			return
		}
		q.Difficulty = n
	}
	if v := c.Query("difficultyMin"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			httpx.Fail(c, httpx.ErrValidation("difficultyMin 必须是 1–100 的整数"))
			return
		}
		q.DifficultyMin = n
	}
	if v := c.Query("difficultyMax"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			httpx.Fail(c, httpx.ErrValidation("difficultyMax 必须是 1–100 的整数"))
			return
		}
		q.DifficultyMax = n
	}
	if q.DifficultyMin > 0 && q.DifficultyMax > 0 && q.DifficultyMin > q.DifficultyMax {
		httpx.Fail(c, httpx.ErrValidation("difficultyMin 不能大于 difficultyMax"))
		return
	}
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	result, err := h.svc.List(c.Request.Context(), v, q, page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

func (h *handler) get(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	v := viewer(c)
	locked, participantAllowed, manager, err := h.lockedProblemAccess(c, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if locked && !manager && !participantAllowed {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	// 只有真正的参赛者访问才临时放开隐藏题；contest.update 管理员不能借此看原本隐藏题。
	if locked && participantAllowed && !manager && !v.CanViewHidden {
		v.CanViewHidden = true
	}
	p, err := h.svc.Get(c.Request.Context(), v, id)
	if err != nil {
		var appErr *httpx.AppError
		if h.contests != nil && errors.As(err, &appErr) && appErr.Code == 40400 {
			if claims, ok := middleware.CurrentClaims(c); ok {
				allowed, checkErr := h.contests.CanViewProblem(c.Request.Context(), claims.UserID, id, time.Now())
				if checkErr != nil {
					httpx.Fail(c, checkErr)
					return
				}
				if allowed {
					v.CanViewHidden = true
					p, err = h.svc.Get(c.Request.Context(), v, id)
				}
			}
		}
		if err != nil {
			httpx.Fail(c, err)
			return
		}
	}
	manageThis := canManageThis(v, p)
	includeSolution := v.CanViewSolution && manageThis
	if !includeSolution && h.accepted != nil {
		if claims, ok := middleware.CurrentClaims(c); ok {
			includeSolution, err = h.accepted.HasAccepted(c.Request.Context(), claims.UserID, p.ID)
			if err != nil {
				httpx.Fail(c, err)
				return
			}
		}
	}
	// 比赛锁定期间，题解只对题解管理员或超管开放。
	if locked && !v.CanViewSolution && !currentIsSuper(c) {
		includeSolution = false
	}
	// 正确答案只对当前题目的编辑者/管理员开放，不能因为拥有
	// problem.update 就读取他人题目的答案。
	includeAnswer := manageThis && (v.CanManageAll || currentIsSuper(c) ||
		currentHas(c, string(rbac.PermProblemUpdate)))
	author, err := h.svc.Author(c.Request.Context(), p.CreatedBy)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	d := problem.ToDetail(p, includeSolution, includeAnswer, manageThis, author)
	d.CanEdit = manageThis
	httpx.OK(c, d)
}

func (h *handler) statistics(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	locked, _, _, err := h.lockedProblemAccess(c, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if locked && !currentCanViewLockedStats(c) {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	st, err := h.svc.Statistics(c.Request.Context(), viewer(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, st)
}

func (h *handler) submissionStats(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	locked, _, _, err := h.lockedProblemAccess(c, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if locked && !currentCanViewLockedStats(c) {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	result, err := h.svc.SubmissionStats(c.Request.Context(), viewer(c), id, problem.SubmissionStatsQuery{
		Sort: c.Query("sort"), Order: c.Query("order"), Page: page, PageSize: size,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.decorateMetricIdentities(c.Request.Context(), result.Items); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

func (h *handler) decorateMetricIdentities(ctx context.Context, items []problem.SubmissionMetric) error {
	if h.identities == nil || len(items) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.UserID)
	}
	found, err := h.identities.Identities(ctx, ids)
	if err != nil {
		return err
	}
	for i := range items {
		identity := found[items[i].UserID]
		items[i].EquippedBadges = identity.Badges
		items[i].EquippedTitle = identity.Title
	}
	return nil
}

func (h *handler) create(c *gin.Context) {
	in, ok := httpx.BindJSON[problem.Input](c)
	if !ok {
		return
	}
	claims, _ := middleware.CurrentClaims(c)
	in, err := in.Normalize()
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	decision, err := h.createDifficultyDecision(c.Request.Context(), in, claims.UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	p, err := h.svc.CreateWithDifficulty(c.Request.Context(), claims.UserID, in, decision)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	d := problem.ToDetail(p, true, true, true, nil)
	d.CanEdit = true
	httpx.Created(c, d)
}

func (h *handler) update(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[problem.Input](c)
	if !ok {
		return
	}
	existing, ok := h.problemForManage(c, id)
	if !ok {
		return
	}
	if in.IsPublic != existing.IsPublic && !middleware.HasPermission(c, string(rbac.PermProblemPublish)) {
		httpx.Fail(c, httpx.ErrForbidden)
		return
	}
	claims, _ := middleware.CurrentClaims(c)
	p, err := h.svc.UpdateAs(c.Request.Context(), id, in, claims.UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	d := problem.ToDetail(p, true, true, true, nil)
	d.CanEdit = true
	httpx.OK(c, d)
}

func (h *handler) delete(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if _, ok := h.problemForManage(c, id); !ok {
		return
	}
	offlined, err := h.svc.Delete(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"offlined": offlined})
}

func (h *handler) listTestcases(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if _, ok := h.problemForManage(c, id); !ok {
		return
	}
	list, err := h.svc.ListTestcases(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, list)
}

func (h *handler) updateTestcase(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	tid, ok := pathID(c, "tid")
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[struct {
		Score    *int  `json:"score"`
		IsSample *bool `json:"isSample"`
	}](c)
	if !ok {
		return
	}
	if _, ok := h.problemForManage(c, id); !ok {
		return
	}
	if err := h.svc.UpdateTestcase(c.Request.Context(), id, tid, in.Score, in.IsSample); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) reorderTestcases(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if _, ok := h.problemForManage(c, id); !ok {
		return
	}
	in, ok := httpx.BindJSON[struct {
		IDs []int64 `json:"ids"`
	}](c)
	if !ok {
		return
	}
	if err := h.svc.ReorderTestcases(c.Request.Context(), id, in.IDs); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) deleteTestcase(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	tid, ok := pathID(c, "tid")
	if !ok {
		return
	}
	if _, ok := h.problemForManage(c, id); !ok {
		return
	}
	if err := h.svc.DeleteTestcase(c.Request.Context(), id, tid); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

// parseMultipart reads the upload form, spooling large files to disk. The
// returned cleanup removes those temporary files.
func parseMultipart(c *gin.Context) (cleanup func(), ok bool) {
	if err := c.Request.ParseMultipartForm(multipartMemory); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.Fail(c, httpx.ErrPayloadTooLarge)
		} else {
			httpx.Fail(c, httpx.ErrBadRequest)
		}
		return nil, false
	}
	return func() { _ = c.Request.MultipartForm.RemoveAll() }, true
}

func formFile(c *gin.Context, field string) (multipart.File, bool) {
	fh, err := c.FormFile(field)
	if err != nil {
		httpx.Fail(c, httpx.ErrValidation("缺少文件字段 "+field))
		return nil, false
	}
	f, err := fh.Open()
	if err != nil {
		httpx.Fail(c, err)
		return nil, false
	}
	return f, true
}

func (h *handler) addTestcase(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if _, ok := h.problemForManage(c, id); !ok {
		return
	}
	cleanup, ok := parseMultipart(c)
	if !ok {
		return
	}
	defer cleanup()

	in, ok := formFile(c, "input")
	if !ok {
		return
	}
	defer func() { _ = in.Close() }()
	out, ok := formFile(c, "output")
	if !ok {
		return
	}
	defer func() { _ = out.Close() }()

	score := defaultTestcaseScore
	if v := c.PostForm("score"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			httpx.Fail(c, httpx.ErrValidation("score 必须是整数"))
			return
		}
		score = n
	}
	isSample := false
	if v := c.PostForm("isSample"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			httpx.Fail(c, httpx.ErrValidation("isSample 必须是 true 或 false"))
			return
		}
		isSample = b
	}

	tc, err := h.svc.AddTestcase(c.Request.Context(), id, problem.TestcaseUpload{
		Input: in, Output: out, Score: score, IsSample: isSample,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, problem.ToTestcaseDTO(tc))
}

func (h *handler) uploadChecker(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if _, ok := h.problemForManage(c, id); !ok {
		return
	}
	cleanup, ok := parseMultipart(c)
	if !ok {
		return
	}
	defer cleanup()

	f, ok := formFile(c, "file")
	if !ok {
		return
	}
	defer func() { _ = f.Close() }()
	if err := h.svc.UploadChecker(c.Request.Context(), id, f); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

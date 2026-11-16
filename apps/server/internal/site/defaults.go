package site

import "time"

// DefaultHomeContent is the canonical home page copy. It mirrors the copy that
// was previously hard-coded in HomeView.vue and is what a brand new deployment
// serves until an administrator edits the page.
func DefaultHomeContent() HomeContent {
	return HomeContent{
		Eyebrow:        "KAPOJ // ONLINE JUDGE",
		Title:          "欢迎来到",
		TitleHighlight: "kapOJ",
		Tagline:        "每一次提交，都是向更高 Rating 迈进一步。",
		Description:    "从第一道题开始，构建你的竞赛档案。用真实的提交、比赛与成长记录，见证每一次进步。",
		PrimaryCTA: Link{
			Icon:        "pi pi-arrow-right",
			IconPos:     "right",
			Title:       "开始刷题",
			Description: "",
			To:          "/problems",
		},
		SecondaryCTA: Link{
			Icon:        "pi pi-trophy",
			IconPos:     "left",
			Title:       "浏览比赛",
			Description: "",
			To:          "/contests",
		},
		QuickTitle: "快速开始",
		QuickLinks: []Link{
			{
				Icon:        "pi pi-book",
				IconPos:     "left",
				Title:       "题库",
				Description: "按难度与标签筛选，从一道适合你的题开始。",
				To:          "/problems",
			},
			{
				Icon:        "pi pi-trophy",
				IconPos:     "left",
				Title:       "比赛",
				Description: "参加 ICPC / IOI / OI 赛制，在实时榜单中验证实力。",
				To:          "/contests",
			},
			{
				Icon:        "pi pi-chart-line",
				IconPos:     "left",
				Title:       "排行榜",
				Description: "查看 Rating、活跃度与贡献榜，找到下一个目标。",
				To:          "/leaderboard",
			},
			{
				Icon:        "pi pi-list-check",
				IconPos:     "left",
				Title:       "测评记录",
				Description: "追踪每次提交的状态、耗时与内存表现。",
				To:          "/submissions",
			},
		},
		GrowthTitle: "成长中心",
		GrowthLinks: []Link{
			{
				Icon:        "pi pi-star",
				IconPos:     "left",
				Title:       "成就墙",
				Description: "解锁荣誉节点，收藏属于你的高光时刻。",
				To:          "/achievements",
			},
			{
				Icon:        "pi pi-check-square",
				IconPos:     "left",
				Title:       "任务大厅",
				Description: "领取限时任务，完成挑战赢取金币奖励。",
				To:          "/tasks",
			},
			{
				Icon:        "pi pi-shop",
				IconPos:     "left",
				Title:       "徽章商城",
				Description: "用金币收藏徽章，组合你的专属展示。",
				To:          "/shop/badges",
			},
			{
				Icon:        "pi pi-crown",
				IconPos:     "left",
				Title:       "称号商店",
				Description: "解锁称号，展示你的竞赛身份与风格。",
				To:          "/shop/titles",
			},
		},
		GuestTitle:       "登录后记录你的成长",
		GuestDescription: "保存每一次提交，积累 Rating，领取任务奖励，并用金币装扮你的徽章与称号。",
	}
}

// DefaultFooterInput is the initial footer configuration used when the
// singleton row is first created.
func DefaultFooterInput() FooterInput {
	return FooterInput{
		StartedAt:  time.Now().UTC(),
		Copyright:  "© kapOJ",
		ShowVisits: true,
	}
}

// DefaultCompanionSettings is the site-wide companion configuration served
// until an administrator overrides it.
func DefaultCompanionSettings() CompanionSettings {
	return CompanionSettings{
		Enabled:              true,
		Name:                 "Kapo",
		DefaultActivityLevel: "normal",
		ContestMute:          true,
		AllowUserDisable:     true,
	}
}

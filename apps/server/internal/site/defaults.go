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
		Description:    "题库、比赛、排行榜、成就与任务已经上线。完成成就与任务赚取金币，再用金币购买并装扮徽章和称号。每日签到还能领取活跃度与金币。",
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
				Title:       "题库",
				Description: "按标签与难度筛选，开始一次练习。",
				To:          "/problems",
			},
			{
				Icon:        "pi pi-trophy",
				Title:       "比赛",
				Description: "参加 ICPC / IOI / OI 赛制，挑战实时榜单。",
				To:          "/contests",
			},
			{
				Icon:        "pi pi-chart-line",
				Title:       "排行榜",
				Description: "查看 Rating、活跃度与共享榜，每日签到领金币。",
				To:          "/leaderboard",
			},
			{
				Icon:        "pi pi-list-check",
				Title:       "测评记录",
				Description: "查看提交状态、耗时与内存。",
				To:          "/submissions",
			},
		},
		GrowthTitle: "成长中心",
		GrowthLinks: []Link{
			{
				Icon:        "pi pi-star",
				Title:       "成就墙",
				Description: "完成条件，解锁成就与奖励。",
				To:          "/achievements",
			},
			{
				Icon:        "pi pi-check-square",
				Title:       "任务大厅",
				Description: "领取任务，AC 全部题目赢取金币。",
				To:          "/tasks",
			},
			{
				Icon:        "pi pi-shop",
				Title:       "徽章商城",
				Description: "用金币购买并装备专属徽章。",
				To:          "/shop/badges",
			},
			{
				Icon:        "pi pi-crown",
				Title:       "称号商店",
				Description: "解锁称号，展示你的竞赛身份。",
				To:          "/shop/titles",
			},
		},
		GuestTitle:       "登录后记录你的成长",
		GuestDescription: "保存提交、领取任务、解锁成就，并用金币装扮徽章与称号。",
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

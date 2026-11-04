package contest

import (
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

type Mode string

const (
	ModeICPC Mode = "icpc"
	ModeIOI  Mode = "ioi"
	ModeOI   Mode = "oi"
)

func (m Mode) Valid() bool {
	return m == ModeICPC || m == ModeIOI || m == ModeOI
}

type RegisterType string

const (
	RegisterOpen     RegisterType = "open"
	RegisterPassword RegisterType = "password"
	RegisterInvite   RegisterType = "invite"
)

func (r RegisterType) Valid() bool {
	return r == RegisterOpen || r == RegisterPassword || r == RegisterInvite
}

type Contest struct {
	ID             int64                `gorm:"primaryKey"`
	Title          string               `gorm:"size:100;not null"`
	DescriptionMD  string               `gorm:"column:description_md;type:text;not null;default:''"`
	StartTime      time.Time            `gorm:"type:timestamptz;not null;index:idx_contests_start"`
	EndTime        time.Time            `gorm:"type:timestamptz;not null;index:idx_contests_end"`
	Mode           Mode                 `gorm:"size:10;not null"`
	IsRated        bool                 `gorm:"not null;default:false"`
	IsPublic       bool                 `gorm:"not null;default:false;index"`
	PasswordHash   string               `gorm:"size:100;not null;default:''"`
	InviteCodeHash string               `gorm:"size:100;not null;default:''"`
	RegisterType   RegisterType         `gorm:"size:20;not null;default:open"`
	CreatedBy      int64                `gorm:"not null;default:0"`
	CreatedAt      time.Time            `gorm:"type:timestamptz;not null"`
	UpdatedAt      time.Time            `gorm:"type:timestamptz;not null"`
	Problems       []ContestProblem     `gorm:"foreignKey:ContestID;constraint:OnDelete:CASCADE" json:"-"`
	Participants   []ContestParticipant `gorm:"foreignKey:ContestID;constraint:OnDelete:CASCADE" json:"-"`
	Announcements  []Announcement       `gorm:"foreignKey:ContestID;constraint:OnDelete:CASCADE" json:"-"`
}

type ContestProblem struct {
	ID        int64           `gorm:"primaryKey"`
	ContestID int64           `gorm:"not null;uniqueIndex:uniq_contest_problems_problem,priority:1;uniqueIndex:uniq_contest_problems_display,priority:1"`
	ProblemID int64           `gorm:"not null;uniqueIndex:uniq_contest_problems_problem,priority:2"`
	DisplayID string          `gorm:"size:1;not null;uniqueIndex:uniq_contest_problems_display,priority:2"`
	SortOrder int             `gorm:"not null;default:1"`
	Score     int             `gorm:"not null;default:100"`
	Problem   problem.Problem `gorm:"foreignKey:ProblemID;constraint:OnDelete:RESTRICT" json:"-"`
}

type ContestParticipant struct {
	ID           int64                `gorm:"primaryKey"`
	ContestID    int64                `gorm:"not null;uniqueIndex:uniq_contest_participants_user,priority:1"`
	UserID       int64                `gorm:"not null;uniqueIndex:uniq_contest_participants_user,priority:2"`
	RegisterTime time.Time            `gorm:"type:timestamptz;not null"`
	ExtraInfo    dbtypes.List[string] `gorm:"column:extra_info_json;type:jsonb;not null;default:'[]'"`
}

type Announcement struct {
	ID        int64     `gorm:"primaryKey"`
	ContestID int64     `gorm:"not null;index:idx_announcements_contest"`
	Title     string    `gorm:"size:100;not null"`
	ContentMD string    `gorm:"column:content_md;type:text;not null;default:''"`
	CreatedBy int64     `gorm:"not null;default:0"`
	CreatedAt time.Time `gorm:"type:timestamptz;not null"`
}

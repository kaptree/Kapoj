package problem

import (
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
)

type Type string

const (
	TypeTraditional Type = "traditional"
	TypeSpecial     Type = "special"
	TypeInteractive Type = "interactive"
)

func (t Type) Valid() bool {
	return t == TypeTraditional || t == TypeSpecial || t == TypeInteractive
}

type Sample struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

type Problem struct {
	ID             int64                `gorm:"primaryKey"`
	Title          string               `gorm:"size:200;not null"`
	Slug           string               `gorm:"size:63;not null;uniqueIndex:uniq_problems_slug"`
	StatementMD    string               `gorm:"column:statement_md;type:text;not null;default:''"`
	InputFormatMD  string               `gorm:"column:input_format_md;type:text;not null;default:''"`
	OutputFormatMD string               `gorm:"column:output_format_md;type:text;not null;default:''"`
	Samples        dbtypes.List[Sample] `gorm:"column:samples_json;type:jsonb;not null;default:'[]'"`
	HintMD         string               `gorm:"column:hint_md;type:text;not null;default:''"`
	SolutionMD     string               `gorm:"column:solution_md;type:text;not null;default:''"`
	SolutionCode   string               `gorm:"type:text;not null;default:''"`
	Type           Type                 `gorm:"size:20;not null;default:traditional"`
	Tags           dbtypes.List[string] `gorm:"column:tags_json;type:jsonb;not null;default:'[]';index:idx_problems_tags,type:gin"`
	Difficulty     int                  `gorm:"not null;default:1;index"`
	TimeLimitMS    int                  `gorm:"column:time_limit_ms;not null;default:1000"`
	MemoryLimitMB  int                  `gorm:"column:memory_limit_mb;not null;default:256"`
	CheckerPath    string               `gorm:"size:255;not null;default:''"`
	IsPublic       bool                 `gorm:"not null;default:false;index"`
	Source         string               `gorm:"size:200;not null;default:''"`
	SubmitCount    int                  `gorm:"not null;default:0"`
	AcceptCount    int                  `gorm:"not null;default:0"`
	CreatedBy      int64                `gorm:"not null;default:0"`
	CreatedAt      time.Time            `gorm:"type:timestamptz;not null"`
	UpdatedAt      time.Time            `gorm:"type:timestamptz;not null"`
	Testcases      []Testcase           `gorm:"foreignKey:ProblemID;constraint:OnDelete:CASCADE" json:"-"`
}

type Testcase struct {
	ID         int64  `gorm:"primaryKey"`
	ProblemID  int64  `gorm:"not null;uniqueIndex:uniq_testcases_problem_order,priority:1"`
	InputFile  string `gorm:"size:255;not null"`
	OutputFile string `gorm:"size:255;not null"`
	InputSize  int64  `gorm:"not null;default:0"`
	OutputSize int64  `gorm:"not null;default:0"`
	Score      int    `gorm:"not null;default:10"`
	IsSample   bool   `gorm:"not null;default:false"`
	SortOrder  int    `gorm:"not null;uniqueIndex:uniq_testcases_problem_order,priority:2"`
}

func (Testcase) TableName() string { return "problem_testcases" }

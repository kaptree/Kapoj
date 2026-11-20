package userimport

import (
	"bytes"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

const (
	maxCSVBytes = 5 << 20
	maxCSVRows  = 5000
)

type rowStatus string

const (
	rowValid    rowStatus = "valid"
	rowConflict rowStatus = "conflict"
	rowInvalid  rowStatus = "invalid"
)

type row struct {
	line      int
	legacyID  int64
	uname     string
	email     string
	hash      string
	salt      string
	hashType  string
	username  string
	generated bool
	status    rowStatus
	reason    string
}

var requiredColumns = []string{"_id", "uname", "mail", "hash", "salt", "hashType"}

func parseHydroCSV(r io.Reader) ([]*row, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxCSVBytes+1))
	if err != nil {
		return nil, httpx.ErrValidation("读取 CSV 失败")
	}
	if len(data) > maxCSVBytes {
		return nil, httpx.ErrValidation("CSV 不能超过 5 MiB")
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, httpx.ErrValidation("CSV 文件为空")
	}
	if !utf8.Valid(data) {
		return nil, httpx.ErrValidation("CSV 必须是 UTF-8 编码")
	}

	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return nil, httpx.ErrValidation("CSV 表头读取失败")
	}
	columns, err := columnIndex(header)
	if err != nil {
		return nil, err
	}

	rows := make([]*row, 0, 64)
	for {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, httpx.ErrValidation("CSV 解析失败，请检查引号与换行")
		}
		if blankRecord(record) {
			continue
		}
		if len(rows) >= maxCSVRows {
			return nil, httpx.ErrValidation("CSV 不能超过 5000 个数据行")
		}

		line := len(rows) + 2
		if len(record) > 0 {
			if l, _ := reader.FieldPos(0); l > 0 {
				line = l
			}
		}
		rows = append(rows, parseRow(record, columns, line))
	}

	markDuplicateRows(rows)
	return rows, nil
}

func columnIndex(header []string) (map[string]int, error) {
	columns := make(map[string]int, len(header))
	for i, name := range header {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			columns[name] = i
		}
	}
	for _, required := range requiredColumns {
		if _, ok := columns[strings.ToLower(required)]; !ok {
			return nil, httpx.ErrValidation("CSV 表头必须包含 _id,uname,mail,hash,salt,hashType")
		}
	}
	return columns, nil
}

func blankRecord(record []string) bool {
	for _, value := range record {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func parseRow(record []string, columns map[string]int, line int) *row {
	value := func(name string) string {
		index, ok := columns[strings.ToLower(name)]
		if !ok || index < 0 || index >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[index])
	}

	item := &row{
		line:     line,
		uname:    value("uname"),
		email:    user.Normalize(value("mail")),
		hash:     strings.ToLower(value("hash")),
		salt:     value("salt"),
		hashType: value("hashType"),
		status:   rowValid,
	}

	if rawID := value("_id"); rawID == "" {
		return invalid(item, "ID 不能为空")
	} else if id, err := strconv.ParseInt(rawID, 10, 64); err != nil || id <= 0 {
		return invalid(item, "ID 必须是正整数")
	} else {
		item.legacyID = id
	}

	if item.uname == "" {
		return invalid(item, "用户名不能为空")
	}
	if count := utf8.RuneCountInString(item.uname); count < 1 || count > 32 {
		return invalid(item, "用户名长度需为 1–32 个字符")
	}
	if strings.ContainsFunc(item.uname, unicode.IsControl) {
		return invalid(item, "用户名不能包含控制字符")
	}

	if err := user.ValidateEmail(item.email); err != nil {
		return invalid(item, "邮箱格式不正确")
	}
	if len(item.hash) != 64 {
		return invalid(item, "密码哈希必须是 64 位十六进制")
	}
	if _, err := hex.DecodeString(item.hash); err != nil {
		return invalid(item, "密码哈希必须是 64 位十六进制")
	}
	if len(item.salt) != 32 {
		return invalid(item, "盐必须是 32 个字符")
	}
	if !strings.EqualFold(item.hashType, "hydro") {
		return invalid(item, "暂不支持该密码哈希类型")
	}

	return item
}

func invalid(item *row, reason string) *row {
	item.status = rowInvalid
	item.reason = reason
	return item
}

func markDuplicateRows(rows []*row) {
	ids := make(map[int64][]*row)
	emails := make(map[string][]*row)
	names := make(map[string][]*row)

	for _, item := range rows {
		if item.legacyID > 0 {
			ids[item.legacyID] = append(ids[item.legacyID], item)
		}
		if item.email != "" {
			emails[item.email] = append(emails[item.email], item)
		}
		normalized := user.Normalize(item.uname)
		if user.ValidateUsername(normalized) == nil {
			names[normalized] = append(names[normalized], item)
		}
	}

	for _, group := range ids {
		if len(group) > 1 {
			for _, item := range group {
				invalid(item, "CSV 内重复的 ID")
			}
		}
	}
	for _, group := range emails {
		if len(group) > 1 {
			for _, item := range group {
				invalid(item, "CSV 内重复的邮箱")
			}
		}
	}
	for _, group := range names {
		if len(group) > 1 {
			for _, item := range group {
				invalid(item, "CSV 内重复的用户名")
			}
		}
	}
}

func applyUsernames(rows []*row, taken map[string]struct{}) {
	if taken == nil {
		taken = make(map[string]struct{})
	}

	for _, item := range rows {
		if item.status != rowValid {
			continue
		}
		normalized := user.Normalize(item.uname)
		if user.ValidateUsername(normalized) != nil {
			continue
		}
		item.username = normalized
		item.generated = false
		taken[normalized] = struct{}{}
	}

	for _, item := range rows {
		if item.status != rowValid || item.username != "" {
			continue
		}
		base := fmt.Sprintf("hydro_%d", item.legacyID)
		candidate := base
		for suffix := 2; ; suffix++ {
			if _, exists := taken[candidate]; !exists {
				break
			}
			candidate = fmt.Sprintf("%s_%d", base, suffix)
		}
		item.username = candidate
		item.generated = true
		taken[candidate] = struct{}{}
	}
}

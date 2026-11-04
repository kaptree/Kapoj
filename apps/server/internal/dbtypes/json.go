package dbtypes

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSON 以 jsonb 文档形式存储任意值。扫描 NULL 列时写入 T 的零值，不返回错误。
type JSON[T any] struct {
	Data T
}

func (j JSON[T]) Value() (driver.Value, error) {
	return json.Marshal(j.Data)
}

func (j *JSON[T]) Scan(src any) error {
	var raw []byte
	switch v := src.(type) {
	case nil:
		var zero T
		j.Data = zero
		return nil
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("dbtypes.JSON: unsupported source type %T", src)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("dbtypes.JSON: %w", err)
	}
	j.Data = out
	return nil
}

// GormDataType 让 GORM 将该列创建为 jsonb。
func (JSON[T]) GormDataType() string { return "jsonb" }

// MarshalJSON 直接输出底层值，避免 API 响应多出一层 "Data" 包装。
func (j JSON[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(j.Data)
}

func (j *JSON[T]) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &j.Data)
}

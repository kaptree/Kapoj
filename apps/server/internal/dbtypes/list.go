// Package dbtypes holds custom column types shared across models.
package dbtypes

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// List stores a slice as a jsonb array. A nil list is written as [] and a
// NULL column is read as an empty, non-nil list.
type List[T any] []T

func (l List[T]) Value() (driver.Value, error) {
	if l == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]T(l))
}

func (l *List[T]) Scan(src any) error {
	var raw []byte
	switch v := src.(type) {
	case nil:
		*l = List[T]{}
		return nil
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("dbtypes.List: unsupported source type %T", src)
	}
	out := List[T]{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("dbtypes.List: %w", err)
	}
	*l = out
	return nil
}

// GormDataType makes GORM create the column as jsonb.
func (List[T]) GormDataType() string { return "jsonb" }

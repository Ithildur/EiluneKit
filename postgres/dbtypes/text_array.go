// Package dbtypes provides database type aliases to keep pq out of business imports.
// dbtypes 提供数据库类型别名，避免业务层直接引入 pq。
package dbtypes

import (
	"database/sql"
	"database/sql/driver"

	"github.com/lib/pq"
)

// TextArray stores a Postgres text array.
// TextArray 保存 Postgres text array。
type TextArray = pq.StringArray

// PQArray adapts an array or slice for SQL values and scanning using pq.Array.
// PQArray 使用 pq.Array 将数组或切片适配为 SQL 值和扫描目标。
func PQArray(a any) interface {
	driver.Valuer
	sql.Scanner
} {
	return pq.Array(a)
}

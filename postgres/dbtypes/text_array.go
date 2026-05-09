// Package dbtypes provides database type aliases to keep pq out of business imports.
// dbtypes 提供数据库类型别名，避免业务层直接引入 pq。
package dbtypes

import "github.com/lib/pq"

// TextArray stores a Postgres text array.
// TextArray 保存 Postgres text array。
type TextArray = pq.StringArray

// PQArray adapts a slice for pq array parameters.
// PQArray 将 slice 适配为 pq array 参数。
var PQArray = pq.Array

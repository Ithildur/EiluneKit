// Package dbtypes provides database type aliases to keep pq out of business imports.
// dbtypes 提供数据库类型别名，避免业务层直接引入 pq。
package dbtypes

import "github.com/lib/pq"

type TextArray = pq.StringArray

var Array = pq.Array

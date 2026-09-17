// Package dsn builds Postgres DSNs.
// Package dsn 构建 Postgres DSN。
package dsn

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Config configures Build.
// Config 配置 Build。
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string
}

// Build returns the Postgres DSN for cfg.
// Build 返回 cfg 的 Postgres DSN。
func Build(cfg Config) (string, error) {
	if cfg.Host == "" || cfg.Port == 0 || cfg.User == "" || cfg.Database == "" {
		return "", fmt.Errorf("incomplete database config")
	}

	host := cfg.Host
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	u := url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(host, strconv.Itoa(cfg.Port)),
		Path:   "/" + cfg.Database,
	}
	if cfg.Password != "" {
		u.User = url.UserPassword(cfg.User, cfg.Password)
	} else {
		u.User = url.User(cfg.User)
	}

	q := u.Query()
	if cfg.SSLMode != "" {
		q.Set("sslmode", cfg.SSLMode)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

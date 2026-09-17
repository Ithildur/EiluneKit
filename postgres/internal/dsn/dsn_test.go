package dsn

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestBuild(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		want    string
		wantErr bool
	}{
		{
			name: "with password and sslmode",
			cfg: Config{
				Host:     "db.example.com",
				Port:     5432,
				User:     "app",
				Password: "secret",
				Database: "main",
				SSLMode:  "require",
			},
			want: "postgres://app:secret@db.example.com:5432/main?sslmode=require",
		},
		{
			name: "without password and sslmode",
			cfg: Config{
				Host:     "db.example.com",
				Port:     5432,
				User:     "app",
				Database: "main",
			},
			want: "postgres://app@db.example.com:5432/main",
		},
		{
			name: "ipv6 loopback",
			cfg:  Config{Host: "::1", Port: 5432, User: "app", Database: "db"},
			want: "postgres://app@[::1]:5432/db",
		},
		{
			name: "bracketed ipv6 loopback",
			cfg:  Config{Host: "[::1]", Port: 5432, User: "app", Database: "db"},
			want: "postgres://app@[::1]:5432/db",
		},
		{
			name: "ipv6",
			cfg:  Config{Host: "2001:db8::1", Port: 5432, User: "app", Database: "db"},
			want: "postgres://app@[2001:db8::1]:5432/db",
		},
		{
			name: "ipv6 zone",
			cfg:  Config{Host: "[fe80::1%eth0]", Port: 5432, User: "app", Database: "db"},
			want: "postgres://app@[fe80::1%25eth0]:5432/db",
		},
		{
			name: "incomplete config",
			cfg: Config{
				Host: "db.example.com",
				Port: 5432,
				User: "app",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Build(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("build dsn: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Build() = %q, want %q", got, tt.want)
			}
			cfg, err := pgx.ParseConfig(got)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Host != strings.Trim(tt.cfg.Host, "[]") || int(cfg.Port) != tt.cfg.Port {
				t.Fatalf("parsed address = %s:%d", cfg.Host, cfg.Port)
			}
		})
	}
}

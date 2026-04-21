package dsn

import "testing"

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
				Name:     "main",
				SSLMode:  "require",
			},
			want: "postgres://app:secret@db.example.com:5432/main?sslmode=require",
		},
		{
			name: "without password and sslmode",
			cfg: Config{
				Host: "db.example.com",
				Port: 5432,
				User: "app",
				Name: "main",
			},
			want: "postgres://app@db.example.com:5432/main",
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
		})
	}
}

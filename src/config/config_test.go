package config

import "testing"

func TestGetURL(t *testing.T) {
	cases := []struct {
		name string
		host string
		port string
		want string
	}{
		{name: "host and port", host: "localhost", port: "8080", want: "localhost:8080"},
		{name: "empty host binds every interface", host: "", port: "8080", want: ":8080"},
		{name: "ipv4", host: "127.0.0.1", port: "3000", want: "127.0.0.1:3000"},
		// JoinHostPort brackets IPv6 literals; naive concatenation would produce
		// an address net.Listen cannot parse.
		{name: "ipv6 is bracketed", host: "::1", port: "8080", want: "[::1]:8080"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := ConfServer{Host: tc.host, Port: tc.port}
			if got := server.GetURL(); got != tc.want {
				t.Fatalf("GetURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNewReadsEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/exchange")
	t.Setenv("SERVER_HOST", "0.0.0.0")
	t.Setenv("SERVER_PORT", "9090")

	c := New()

	if c.DB.URL != "postgres://user:pass@localhost:5432/exchange" {
		t.Fatalf("DB.URL = %q", c.DB.URL)
	}
	if c.Server.Host != "0.0.0.0" {
		t.Fatalf("Server.Host = %q, want 0.0.0.0", c.Server.Host)
	}
	if c.Server.Port != "9090" {
		t.Fatalf("Server.Port = %q, want 9090", c.Server.Port)
	}
	if got := c.Server.GetURL(); got != "0.0.0.0:9090" {
		t.Fatalf("GetURL() = %q, want 0.0.0.0:9090", got)
	}
}

// A missing .env is explicitly not an error — the deployed process gets its
// configuration from the real environment. Unset variables come back empty
// rather than defaulted, which is why a misconfigured DATABASE_URL surfaces as
// a connection failure at startup instead of here.
func TestNewTreatsUnsetVariablesAsEmpty(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("SERVER_HOST", "")
	t.Setenv("SERVER_PORT", "")

	c := New()

	if c.DB.URL != "" {
		t.Fatalf("DB.URL = %q, want empty", c.DB.URL)
	}
	if c.Server.Host != "" || c.Server.Port != "" {
		t.Fatalf("server = %q:%q, want both empty", c.Server.Host, c.Server.Port)
	}
}

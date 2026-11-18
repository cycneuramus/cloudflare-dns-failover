package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// resetGlobals gives parseConfig a clean pflag/viper state and fake CLI args,
// since it reads from package-level singletons.
func resetGlobals(t *testing.T, args ...string) {
	t.Helper()

	oldArgs := os.Args
	oldFlags := pflag.CommandLine

	os.Args = append([]string{"cloudflare-dns-failover"}, args...)
	pflag.CommandLine = pflag.NewFlagSet("test", pflag.ContinueOnError)
	viper.Reset()

	t.Cleanup(func() {
		os.Args = oldArgs
		pflag.CommandLine = oldFlags
		viper.Reset()
	})
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

func chdir(t *testing.T, dir string) {
	t.Helper()

	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

const validConfig = `
api_key: "my-api-key"
check_interval: 10
dns_records:
  - domain: "*.mydomain.com"
    zone_id: "zone-1"
    proxied: false
    ips:
      - "10.10.10.10"
      - "10.10.10.11"
  - domain: "mydomain.com"
    zone_id: "zone-2"
    proxied: true
    ips:
      - "10.10.10.13"
`

func assertValidConfig(t *testing.T, cfg Config) {
	t.Helper()

	if cfg.APIKey != "my-api-key" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "my-api-key")
	}
	if cfg.CheckInterval != 10 {
		t.Errorf("CheckInterval = %d, want 10", cfg.CheckInterval)
	}
	if len(cfg.Records) != 2 {
		t.Fatalf("got %d records, want 2", len(cfg.Records))
	}

	first, second := cfg.Records[0], cfg.Records[1]

	if first.Domain != "*.mydomain.com" || first.ZoneID != "zone-1" || first.Proxied {
		t.Errorf("unexpected first record: %+v", first)
	}
	if len(first.IPs) != 2 || first.IPs[0] != "10.10.10.10" || first.IPs[1] != "10.10.10.11" {
		t.Errorf("unexpected first record IPs: %v", first.IPs)
	}

	if second.Domain != "mydomain.com" || second.ZoneID != "zone-2" || !second.Proxied {
		t.Errorf("unexpected second record: %+v", second)
	}
	if len(second.IPs) != 1 || second.IPs[0] != "10.10.10.13" {
		t.Errorf("unexpected second record IPs: %v", second.IPs)
	}
}

func TestParseConfig_ConfigFlag(t *testing.T) {
	tests := []struct {
		name string
		args func(path string) []string
	}{
		{"long flag with equals", func(p string) []string { return []string{"--config=" + p} }},
		{"long flag with space", func(p string) []string { return []string{"--config", p} }},
		{"short flag", func(p string) []string { return []string{"-c", p} }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeFile(t, t.TempDir(), "custom.yml", validConfig)
			resetGlobals(t, tc.args(path)...)

			cfg, err := parseConfig()
			if err != nil {
				t.Fatalf("parseConfig() error: %v", err)
			}
			assertValidConfig(t, cfg)
		})
	}
}

func TestParseConfig_DefaultsToConfigInWorkingDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "config.yml", validConfig)
	chdir(t, dir)
	resetGlobals(t)

	cfg, err := parseConfig()
	if err != nil {
		t.Fatalf("parseConfig() error: %v", err)
	}
	assertValidConfig(t, cfg)
}

func TestParseConfig_NoConfigInWorkingDir(t *testing.T) {
	chdir(t, t.TempDir())
	resetGlobals(t)

	_, err := parseConfig()
	if err == nil {
		t.Fatal("expected an error when no config file exists")
	}
	if !strings.Contains(err.Error(), "Error reading config") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseConfig_Errors(t *testing.T) {
	tests := []struct {
		name    string
		content string // ignored when missingFile is true
		missing bool
		wantErr string
	}{
		{
			name:    "missing file",
			missing: true,
			wantErr: "Error reading config",
		},
		{
			name:    "malformed yaml",
			content: "api_key: [unclosed",
			wantErr: "Error reading config",
		},
		{
			name:    "check_interval wrong type",
			content: "api_key: k\ncheck_interval: not-a-number\n",
			wantErr: "Unable to decode config",
		},
		{
			name:    "dns_records wrong type",
			content: "api_key: k\ncheck_interval: 5\ndns_records: nope\n",
			wantErr: "Unable to decode config",
		},
		{
			name:    "check_interval missing",
			content: "api_key: k\n",
			wantErr: "check_interval must be a positive number",
		},
		{
			name:    "check_interval zero",
			content: "api_key: k\ncheck_interval: 0\n",
			wantErr: "check_interval must be a positive number",
		},
		{
			name:    "check_interval negative",
			content: "api_key: k\ncheck_interval: -5\n",
			wantErr: "check_interval must be a positive number",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yml")
			if !tc.missing {
				path = writeFile(t, dir, "config.yml", tc.content)
			}
			resetGlobals(t, "-c", path)

			_, err := parseConfig()
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestParseConfig_ExampleFileIsValid(t *testing.T) {
	path := "config.yml.example"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("%s not found: %v", path, err)
	}

	// viper infers the format from the extension, so copy to a .yml file.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	copyPath := writeFile(t, t.TempDir(), "example.yml", string(data))
	resetGlobals(t, "-c", copyPath)

	cfg, err := parseConfig()
	if err != nil {
		t.Fatalf("example config should parse cleanly: %v", err)
	}
	if len(cfg.Records) == 0 {
		t.Error("example config should contain at least one record")
	}
}

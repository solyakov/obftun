package config

import (
	"os"
	"strings"
	"testing"

	"github.com/jessevdk/go-flags"
)

func clearEnv(t *testing.T) {
	t.Helper()
	envs := []string{
		"OBFTUN_BIND", "OBFTUN_DIAL", "OBFTUN_IFACE", "OBFTUN_SCRIPT",
		"OBFTUN_SCRIPT_TIMEOUT", "OBFTUN_VERBOSE", "OBFTUN_SECRET", "OBFTUN_MAX_CLIENTS",
	}
	for _, e := range envs {
		prev, had := os.LookupEnv(e)
		os.Unsetenv(e)
		name := e
		if had {
			t.Cleanup(func() { os.Setenv(name, prev) })
		} else {
			t.Cleanup(func() { os.Unsetenv(name) })
		}
	}
}

func testParse(args ...string) (*Config, error) {
	return parseArgs(args, flags.HelpFlag|flags.PassDoubleDash)
}

func TestParseHappyServer(t *testing.T) {
	clearEnv(t)

	cfg, err := testParse("--secret", "long-enough-secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.IsServer() {
		t.Fatal("expected server mode with empty --dial")
	}
	if cfg.MaxClients != 10 {
		t.Fatalf("expected default MaxClients=10, got %d", cfg.MaxClients)
	}
	if cfg.ScriptTimeout != 15 {
		t.Fatalf("expected default ScriptTimeout=15, got %d", cfg.ScriptTimeout)
	}
}

func TestParseHappyClient(t *testing.T) {
	clearEnv(t)

	cfg, err := testParse("--secret", "long-enough-secret", "--dial", "example.com:80")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.IsServer() {
		t.Fatal("expected client mode with --dial set")
	}
}

func TestParseShortSecretRejected(t *testing.T) {
	clearEnv(t)

	_, err := testParse("--secret", "short")
	if err == nil {
		t.Fatal("expected error for short secret")
	}
	if !strings.Contains(err.Error(), "secret") {
		t.Fatalf("expected secret error, got %v", err)
	}
}

func TestParseZeroMaxClientsRejected(t *testing.T) {
	clearEnv(t)

	_, err := testParse("--secret", "long-enough-secret", "--max-clients", "0")
	if err == nil {
		t.Fatal("expected error for zero --max-clients")
	}
	if !strings.Contains(err.Error(), "max-clients") {
		t.Fatalf("expected max-clients error, got %v", err)
	}
}

func TestParseZeroScriptTimeoutRejected(t *testing.T) {
	clearEnv(t)

	_, err := testParse("--secret", "long-enough-secret", "--script-timeout", "0")
	if err == nil {
		t.Fatal("expected error for zero --script-timeout")
	}
	if !strings.Contains(err.Error(), "script-timeout") {
		t.Fatalf("expected script-timeout error, got %v", err)
	}
}

func TestParseServerOnlyFlagOnClientRejected(t *testing.T) {
	clearEnv(t)

	_, err := testParse(
		"--secret", "long-enough-secret",
		"--dial", "example.com:80",
		"--max-clients", "5",
	)
	if err == nil {
		t.Fatal("expected error for --max-clients in client mode")
	}
	if !strings.Contains(err.Error(), "server-only") {
		t.Fatalf("expected server-only error, got %v", err)
	}
}

func TestParseSecretRequired(t *testing.T) {
	clearEnv(t)

	_, err := testParse()
	if err == nil {
		t.Fatal("expected error when --secret missing")
	}
}

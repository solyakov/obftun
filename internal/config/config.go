package config

import (
	"fmt"
	"os"

	"github.com/jessevdk/go-flags"
)

const minSecretLen = 8

type Config struct {
	Bind          string `short:"b" long:"bind" env:"OBFTUN_BIND" description:"Bind address (server only)" default:":80"`
	Dial          string `short:"d" long:"dial" env:"OBFTUN_DIAL" description:"Connect to remote server (client only)"`
	Iface         string `short:"i" long:"iface" env:"OBFTUN_IFACE" description:"Interface name pattern" default:"tap%d"`
	Script        string `short:"s" long:"script" env:"OBFTUN_SCRIPT" description:"Script to setup the tunnel interface"`
	ScriptTimeout int    `short:"t" long:"script-timeout" env:"OBFTUN_SCRIPT_TIMEOUT" description:"Script execution timeout in seconds" default:"15"`
	Verbose       bool   `short:"v" long:"verbose" env:"OBFTUN_VERBOSE" description:"Verbose output"`
	Secret        string `short:"x" long:"secret" env:"OBFTUN_SECRET" description:"Shared secret for encryption" required:"true"`
	MaxClients    int    `short:"m" long:"max-clients" env:"OBFTUN_MAX_CLIENTS" description:"Maximum concurrent clients (server only)" default:"10"`
}

var serverOnlyFlags = []string{"bind", "max-clients"}

func Parse() (*Config, error) {
	return parseArgs(os.Args[1:], flags.Default)
}

func parseArgs(args []string, opts flags.Options) (*Config, error) {
	var cfg Config
	parser := flags.NewParser(&cfg, opts)

	if _, err := parser.ParseArgs(args); err != nil {
		return nil, err
	}

	if !cfg.IsServer() {
		for _, name := range serverOnlyFlags {
			opt := parser.FindOptionByLongName(name)
			if opt != nil && opt.IsSet() && !opt.IsSetDefault() {
				return nil, fmt.Errorf("--%s is a server-only option", name)
			}
		}
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) IsServer() bool {
	return c.Dial == ""
}

func (c *Config) validate() error {
	if len(c.Secret) < minSecretLen {
		return fmt.Errorf("--secret must be at least %d characters", minSecretLen)
	}
	if c.ScriptTimeout <= 0 {
		return fmt.Errorf("--script-timeout must be positive")
	}
	if c.IsServer() && c.MaxClients <= 0 {
		return fmt.Errorf("--max-clients must be positive")
	}
	return nil
}

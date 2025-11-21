package config

import (
	"fmt"

	"github.com/jessevdk/go-flags"
)

type Config struct {
	Bind          string `short:"b" long:"bind" env:"OBFTUN_BIND" description:"Bind address (optional for client, required for server)"`
	Dial          string `short:"d" long:"dial" env:"OBFTUN_DIAL" description:"Connect to remote server (required for client)"`
	Iface         string `short:"i" long:"iface" env:"OBFTUN_IFACE" description:"Interface name pattern" default:"tap%d"`
	Script        string `short:"s" long:"script" env:"OBFTUN_SCRIPT" description:"Script to setup the tunnel interface"`
	ScriptTimeout int    `short:"t" long:"script-timeout" env:"OBFTUN_SCRIPT_TIMEOUT" description:"Script execution timeout in seconds" default:"15"`
	ReadTimeout   int    `short:"r" long:"read-timeout" env:"OBFTUN_READ_TIMEOUT" description:"Connection read timeout in seconds" default:"60"`
	Verbose       bool   `short:"v" long:"verbose" env:"OBFTUN_VERBOSE" description:"Verbose output"`
	Certificate   string `short:"c" long:"certificate" env:"OBFTUN_CERTIFICATE" description:"Certificate" default:"cert.crt"`
	Key           string `short:"k" long:"key" env:"OBFTUN_KEY" description:"Private key" default:"key.pem"`
	CA            string `short:"a" long:"ca" env:"OBFTUN_CA" description:"CA certificate" default:"ca.crt"`
}

func Parse() (*Config, error) {
	var cfg Config
	if _, err := flags.Parse(&cfg); err != nil {
		return nil, err
	}

	if cfg.Dial == "" && cfg.Bind == "" {
		return nil, fmt.Errorf("either --dial or --bind must be specified")
	}

	return &cfg, nil
}

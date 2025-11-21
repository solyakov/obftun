package script

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"time"

	"github.com/asolyakov/obftun/internal/config"
)

func Run(cfg *config.Config, ifaceName, action, peerAddr string) error {
	if cfg.Script == "" {
		return nil
	}

	timeout := time.Duration(cfg.ScriptTimeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, cfg.Script, ifaceName, action, peerAddr)
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		log.Println(string(output))
	}
	if err != nil {
		return fmt.Errorf("failed to execute script %s with action %s and peer addr %s: %w", cfg.Script, action, peerAddr, err)
	}
	return nil
}

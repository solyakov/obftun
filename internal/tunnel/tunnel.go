package tunnel

import (
	"errors"
	"fmt"

	"github.com/asolyakov/obftun/internal/config"
	"github.com/asolyakov/obftun/internal/script"
	"github.com/songgao/water"
)

const (
	actionUp   = "up"
	actionDown = "down"
)

type Interface struct {
	*water.Interface
	cfg      *config.Config
	peerAddr string
}

func New(cfg *config.Config, peerAddr string) (*Interface, error) {
	waterConfig := water.Config{
		DeviceType: water.TAP,
		PlatformSpecificParams: water.PlatformSpecificParams{
			Name: cfg.Iface,
		},
	}

	iface, err := water.New(waterConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create tunnel interface: %w", err)
	}

	tun := &Interface{
		Interface: iface,
		cfg:       cfg,
		peerAddr:  peerAddr,
	}

	if err := script.Run(cfg, iface.Name(), actionUp, peerAddr); err != nil {
		iface.Close()
		return nil, fmt.Errorf("failed to run script: %w", err)
	}

	return tun, nil
}

func (i *Interface) Close() error {
	scriptErr := script.Run(i.cfg, i.Name(), actionDown, i.peerAddr)
	closeErr := i.Interface.Close()
	return errors.Join(scriptErr, closeErr)
}

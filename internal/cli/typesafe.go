package cli

import (
	"github.com/ucpr/craft-auto-routing/internal/config"
	"github.com/ucpr/craft-auto-routing/internal/typesafe"
)

func typesafeClient(cfg config.Config) *typesafe.Client {
	return typesafe.NewClient(cfg.TypesafeAPIKey, cfg.TypesafeModel)
}

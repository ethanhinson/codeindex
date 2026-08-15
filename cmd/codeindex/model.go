package main

import (
	"fmt"
	"os"

	"codeindex/internal/config"
	"codeindex/internal/embed"
)

// runModel implements `codeindex model <pull|use|status>` — embedding-weight
// management. The bundled model always works; pull/use switch a repo to a
// larger one (which takes precedence, invalidating stored vectors on the next
// build via the model stamp).
func runModel(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: codeindex model <pull <name|url> | use <name> [repo] | status [repo]>")
	}
	switch args[0] {
	case "pull":
		if len(args) < 2 {
			return fmt.Errorf("usage: codeindex model pull <name|url>  (known: %v)", embed.RegistryNames())
		}
		_, err := embed.Pull(args[1], os.Stdout)
		return err
	case "use":
		if len(args) < 2 {
			return fmt.Errorf("usage: codeindex model use <name|''> [repo]")
		}
		root := "."
		if len(args) >= 3 {
			root = args[2]
		}
		cfg, err := config.Load(root)
		if err != nil {
			return err
		}
		cfg.Embed.Model = args[1]
		if cfg.Embed.Provider == "" {
			cfg.Embed.Provider = "local"
		}
		if err := config.Save(root, cfg); err != nil {
			return err
		}
		fmt.Printf("repo %s now embeds with %q (bundled default: %s)\n", root, args[1], embed.BundledModelName)
		fmt.Println("stored vectors re-embed on the next build (model stamp mismatch)")
		return nil
	case "status":
		root := "."
		if len(args) >= 2 {
			root = args[1]
		}
		cfg, err := config.Load(root)
		if err != nil {
			return err
		}
		active := cfg.Embed.Model
		if active == "" {
			active = embed.BundledModelName + " (bundled)"
		}
		fmt.Printf("provider: %s\nactive model: %s\n", orDefault(cfg.Embed.Provider, "local"), active)
		cached, err := embed.CachedModels()
		if err == nil && len(cached) > 0 {
			fmt.Println("cached models:")
			for _, m := range cached {
				fmt.Printf("  %s\n", m)
			}
		}
		fmt.Printf("pullable: %v\n", embed.RegistryNames())
		return nil
	default:
		return fmt.Errorf("unknown model verb %q (pull|use|status)", args[0])
	}
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

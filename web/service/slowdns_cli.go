package service

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database"
)

// RunSlowDNSCommand is used by the binary and management menu; no build tools,
// separate dnstt process or generated on-disk private key are needed at runtime.
func RunSlowDNSCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: x-ui slowdns status|install|configure|start|stop|uninstall|regenerate-key [options]")
	}
	command := flag.NewFlagSet("slowdns "+args[0], flag.ContinueOnError)
	host := command.String("host-domain", "", "nameserver host with an A/AAAA record")
	zone := command.String("ns-domain", "", "DNS zone delegated to the nameserver host")
	listen := command.String("listen", "", "internal UDP listener (Linux default: dual-stack :5301, public DNS port 53)")
	mtu := command.Int("mtu", 0, "DNS response size, 512-4096 (first install: 1232; otherwise keep saved value)")
	jsonOutput := command.Bool("json", false, "print public configuration as JSON")
	defaults := command.Bool("defaults", false, "print menu defaults (host, NS zone, listener, MTU)")
	summary := command.Bool("summary", false, "print compact menu status")
	details := command.Bool("details", false, "print the management details page")
	confirm := command.Bool("yes", false, "confirm replacing the SlowDNS keypair")
	if err := command.Parse(args[1:]); err != nil {
		return err
	}
	if command.NArg() != 0 {
		return errors.New("unexpected SlowDNS command arguments")
	}
	if err := database.InitDB(config.GetDBPath()); err != nil {
		return err
	}
	defer database.CloseDB()
	var service SlowDNSService
	cfg, err := service.GetConfig()
	if err != nil {
		return err
	}
	changed := true
	switch args[0] {
	case "status":
		changed = false
	case "install", "configure":
		address := *listen
		if address == "" {
			address = cfg.Listen
			if address == "" {
				address = DefaultSlowDNSListen()
			}
		}
		if err := checkSlowDNSRoutingRequirements(address); err != nil {
			return err
		}
		cfg, err = service.Configure(*host, *zone, *listen, *mtu, args[0] == "install")
	case "start", "stop":
		cfg, err = service.SetEnabled(args[0] == "start")
	case "uninstall":
		cfg, err = service.Uninstall()
	case "regenerate-key":
		if !*confirm {
			return errors.New("regenerating revokes the old public key; add --yes to confirm")
		}
		cfg, err = service.RegenerateKey()
	default:
		return fmt.Errorf("unknown SlowDNS command %q", args[0])
	}
	if err != nil {
		return err
	}
	if changed {
		if err := os.Chmod(config.GetDBPath(), 0600); err != nil {
			return fmt.Errorf("protect SlowDNS database: %w", err)
		}
		if err := WriteSlowDNSRuntimeConfig(); err != nil {
			return err
		}
		if err := SyncSlowDNSRouting(); err != nil {
			return err
		}
	}
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(cfg)
	}
	if *details {
		state := "Not installed"
		if cfg.Installed {
			state = "Stopped"
			if cfg.Enabled {
				state = "Enabled"
			}
		}
		fmt.Printf("\nSlowDNS\nStatus: %s\n", state)
		if cfg.Installed {
			fmt.Printf("NameServer: %s\nUDP listener: %s\nPublic DNS port: 53 / UDP\nMTU: %d\n", cfg.NSDomain, cfg.Listen, cfg.MTU)
			fmt.Printf("\nPublic Key:\n%s\n", cfg.PublicKey)
			fmt.Printf("\nDNS Records:\nA/AAAA %s -> Server IP\nNS %s -> %s\n", cfg.HostDomain, cfg.NSDomain, cfg.HostDomain)
		}
		fmt.Println()
		return nil
	}
	if *summary {
		state := "Not installed"
		if cfg.Installed {
			state = "Stopped"
			if cfg.Enabled {
				state = "Enabled"
			}
		}
		fmt.Printf("  Status: %s\n", state)
		if cfg.Installed {
			fmt.Printf("  NameServer: %s\n  MTU: %d  |  Public DNS: 53/UDP\n", cfg.NSDomain, cfg.MTU)
		}
		return nil
	}
	if *defaults {
		listen := cfg.Listen
		if listen == "" {
			listen = DefaultSlowDNSListen()
		}
		fmt.Printf("%s\n%s\n%s\n%d\n", cfg.HostDomain, cfg.NSDomain, listen, cfg.MTU)
		return nil
	}
	fmt.Printf("Installed: %t\nEnabled: %t\n", cfg.Installed, cfg.Enabled)
	if cfg.Installed {
		fmt.Printf("Host domain: %s\nNS domain: %s\nUDP listener: %s\nMTU: %d\nPublic key: %s\n", cfg.HostDomain, cfg.NSDomain, cfg.Listen, cfg.MTU, cfg.PublicKey)
		fmt.Println("Public DNS port: 53 / UDP")
		fmt.Printf("DNS records:\n  A/AAAA %s -> the corresponding listening server IP\n  NS %s -> %s\n", cfg.HostDomain, cfg.NSDomain, cfg.HostDomain)
		fmt.Println("NameServer: " + cfg.NSDomain)
	}
	if changed {
		fmt.Println("Configuration saved. Restart Xray to apply it. Existing keys are retained unless explicitly regenerated.")
	}
	return nil
}

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jarviisha/codohue/internal/config"
	"github.com/jarviisha/codohue/internal/core/access"
	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	infrapg "github.com/jarviisha/codohue/internal/infra/postgres"
	"github.com/jarviisha/codohue/internal/nsconfig"
)

// runAccessCLI is a local operator tool: database credentials confer authority.
// Passwords and tokens are accepted only through files, never argv or stdout.
func runAccessCLI(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: admin access bootstrap|recover|token|revoke-token|provision [flags]")
	}
	fs := flag.NewFlagSet("access", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	username := fs.String("username", "", "operator username")
	secretFile := fs.String("secret-file", "", "password or token file")
	name := fs.String("name", "", "service token or namespace name")
	permissions := fs.String("permissions", "", "comma-separated permissions")
	namespaces := fs.String("namespaces", "", "comma-separated namespaces")
	configFile := fs.String("config-file", "", "namespace config JSON file")
	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("invalid access arguments (use documented flags)")
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	switch args[0] {
	case "bootstrap", "recover", "token", "revoke-token", "provision":
	default:
		return fmt.Errorf("unknown access command")
	}
	secret := ""
	if args[0] != "revoke-token" {
		b, err := os.ReadFile(*secretFile)
		if err != nil {
			return fmt.Errorf("read secret file: %w", err)
		}
		secret = strings.TrimRight(string(b), "\r\n")
		if secret == "" {
			return fmt.Errorf("empty secret file")
		}
	}
	cfg, err := config.LoadCron()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db, err := infrapg.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	store := access.NewStore(db)
	switch args[0] {
	case "bootstrap":
		return store.Bootstrap(ctx, *username, secret)
	case "recover":
		return store.SetAccount(ctx, "local-recovery", *username, secret, "owner", false, true)
	case "token":
		return store.ProvisionToken(ctx, "local-operator", *name, secret, strings.Split(*permissions, ","), strings.Split(*namespaces, ","))
	case "revoke-token":
		return store.RevokeToken(ctx, "local-operator", *name)
	case "provision":
		if *name == "" {
			return fmt.Errorf("namespace name required")
		}
		req := &nsconfig.UpsertRequest{}
		if *configFile != "" {
			f, err := os.Open(*configFile)
			if err != nil {
				return fmt.Errorf("open namespace config: %w", err)
			}
			defer f.Close() //nolint:errcheck // read-only configuration file; decoding errors are checked
			d := json.NewDecoder(f)
			d.DisallowUnknownFields()
			if err = d.Decode(req); err != nil {
				return fmt.Errorf("invalid namespace config JSON")
			}
			if d.Decode(new(any)) != io.EOF {
				return fmt.Errorf("namespace config must contain one JSON object")
			}
		}
		req.ProvisionAPIKey = secret
		locker, err := nslifecycle.NewPostgresLocker(db)
		if err != nil {
			return err
		}
		defer locker.Close()
		service := nsconfig.NewService(nsconfig.NewRepository(db))
		service.SetLifecycleCoordinator(nslifecycle.NewService(nslifecycle.NewRepository(db), locker))
		if err := store.Audit(ctx, "local-operator", "namespace.provision", *name); err != nil {
			return err
		}
		_, err = service.Upsert(ctx, *name, req)
		return err
	}
	return nil
}

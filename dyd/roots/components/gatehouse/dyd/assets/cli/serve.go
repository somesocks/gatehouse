package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	clib "gatehouse/cli-builder"
	"gatehouse/config"
	"gatehouse/configschema"
	"gatehouse/database"
	"gatehouse/keychain"
)

var serveCommand = clib.NewCommand("serve", "run the Gatehouse daemon").
	WithOption(clib.NewOption("config", "path to a YAML or JSON configuration file")).
	WithAction(func(request clib.ActionRequest) int {
		document := configschema.GatehouseConfig{ApiVersion: "v1"}
		var databaseConfig config.DatabaseConfig
		var state config.State
		var warnings []config.Warning
		var err error

		if path, ok := request.Opts["config"].(string); ok {
			err, document = config.ValidateFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "configuration is invalid: %s: %v\n", path, err)
				return 1
			}
			err, databaseConfig, warnings = config.ResolveDatabase(document)
		} else {
			err, databaseConfig, warnings = config.ResolveDatabase(document)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "configure database: %v\n", err)
			return 1
		}
		for _, warning := range warnings {
			fmt.Fprintf(os.Stderr, "warning: %s\n", warning.Message)
		}
		err, state = config.ResolveState(document)
		if err != nil {
			fmt.Fprintf(os.Stderr, "configure state: %v\n", err)
			return 1
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		err, store := database.Open(ctx, databaseConfig)
		if err != nil {
			fmt.Fprintf(os.Stderr, "start database: %v\n", err)
			return 1
		}
		defer store.Close()
		err, migrations := database.BuildMigrations(databaseConfig, state)
		if err != nil {
			fmt.Fprintf(os.Stderr, "generate migrations: %v\n", err)
			return 1
		}
		if err := database.Migrate(ctx, store, migrations); err != nil {
			fmt.Fprintf(os.Stderr, "migrate database: %v\n", err)
			return 1
		}
		err, keyring := keychain.Prepare(ctx, store, state.Keychains, keychain.NewPassphraseSourceResolver())
		if err != nil {
			fmt.Fprintf(os.Stderr, "prepare keychains: %v\n", err)
			return 1
		}
		defer keyring.Close()

		fmt.Fprintln(os.Stderr, "Gatehouse is serving")
		<-ctx.Done()
		return 0
	})

package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"
	_ "github.com/dbos-inc/dbos-transact-golang/dbos/driver/sqlite"

	"gatehouse/agent"
	"gatehouse/auth"
	clib "gatehouse/cli-builder"
	"gatehouse/config"
	"gatehouse/configschema"
	"gatehouse/database"
	"gatehouse/diagnostics"
	"gatehouse/httpservice"
	"gatehouse/keychain"
	"gatehouse/migrations"
)

var serveCommand = clib.NewCommand("serve", "run the Gatehouse daemon").
	WithOption(clib.NewOption("config", "path to a YAML or JSON configuration file")).
	WithAction(func(request clib.ActionRequest) int {
		document := configschema.GatehouseConfig{ApiVersion: "v1"}
		var databaseConfig config.DatabaseConfig
		var state config.State
		var services config.Services
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
		err, services = config.ResolveServices(document)
		if err != nil {
			fmt.Fprintf(os.Stderr, "configure services: %v\n", err)
			return 1
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		diagnosticSignals := diagnostics.Signals()
		if len(diagnosticSignals) > 0 {
			diagnosticEvents := make(chan os.Signal, len(diagnosticSignals))
			signal.Notify(diagnosticEvents, diagnosticSignals...)
			defer signal.Stop(diagnosticEvents)
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case received := <-diagnosticEvents:
						if err := diagnostics.EmitSignal(received, os.Stdout, os.Stderr); err != nil {
							fmt.Fprintf(os.Stderr, "emit diagnostics metrics: %v\n", err)
						}
					}
				}
			}()
		}

		err, store := database.Open(ctx, databaseConfig)
		if err != nil {
			fmt.Fprintf(os.Stderr, "start database: %v\n", err)
			return 1
		}
		defer store.Close()
		err, keyring := keychain.NewKeyring(store, state.Keychains, keychain.NewPassphraseSourceResolver())
		if err != nil {
			fmt.Fprintf(os.Stderr, "prepare keychain passphrases: %v\n", err)
			return 1
		}
		defer keyring.Close()
		err, set := migrations.Build(databaseConfig, state, keyring)
		if err != nil {
			fmt.Fprintf(os.Stderr, "generate migrations: %v\n", err)
			return 1
		}
		if err := migrations.Run(ctx, store, set); err != nil {
			fmt.Fprintf(os.Stderr, "migrate database: %v\n", err)
			return 1
		}
		dbosContext, err := dbos.NewContext(ctx, dbos.Config{
			AppName:        "gatehouse",
			SQLiteSystemDB: store.DB,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "start DBOS: %v\n", err)
			return 1
		}
		err, replies := agent.NewSessionEventReplyRuntime(dbosContext, store, keyring)
		if err != nil {
			fmt.Fprintf(os.Stderr, "prepare agent replies: %v\n", err)
			return 1
		}
		defer func() {
			if err := dbos.Shutdown(dbosContext, 10*time.Second); err != nil {
				fmt.Fprintf(os.Stderr, "stop DBOS: %v\n", err)
			}
		}()
		if err := dbos.Launch(dbosContext); err != nil {
			fmt.Fprintf(os.Stderr, "launch DBOS: %v\n", err)
			return 1
		}
		reconcileContext, stopReconciliation := context.WithCancel(ctx)
		reconciliationDone := make(chan struct{})
		go func() {
			defer close(reconciliationDone)
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				if err := replies.Reconcile(); err != nil {
					fmt.Fprintf(os.Stderr, "reconcile agent replies: %v\n", err)
				}
				select {
				case <-reconcileContext.Done():
					return
				case <-ticker.C:
				}
			}
		}()
		defer func() {
			stopReconciliation()
			<-reconciliationDone
		}()
		if services.HTTP == nil || !services.HTTP.Enabled {
			fmt.Fprintln(os.Stderr, "Gatehouse is serving")
			<-ctx.Done()
			return 0
		}
		err, tokens := auth.Prepare(ctx, store, keyring, services.HTTP.Keychain)
		if err != nil {
			fmt.Fprintf(os.Stderr, "prepare bearer tokens: %v\n", err)
			return 1
		}

		err, service := httpservice.StartWithReplyDispatcher(*services.HTTP, store, replies, tokens)
		if err != nil {
			fmt.Fprintf(os.Stderr, "start HTTP service: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "Gatehouse is serving at http://%s\n", service.Address())

		select {
		case <-ctx.Done():
			shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := service.Shutdown(shutdownContext); err != nil {
				fmt.Fprintf(os.Stderr, "stop HTTP service: %v\n", err)
				return 1
			}
			if err := <-service.Done(); err != nil {
				fmt.Fprintf(os.Stderr, "serve HTTP service: %v\n", err)
				return 1
			}
		case err := <-service.Done():
			if err != nil {
				fmt.Fprintf(os.Stderr, "serve HTTP service: %v\n", err)
				return 1
			}
		}
		return 0
	})

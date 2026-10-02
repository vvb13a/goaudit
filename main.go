package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vvb13a/goaudit/api"
	"github.com/vvb13a/goaudit/checks"
	"github.com/vvb13a/goaudit/service"
	"github.com/vvb13a/goaudit/store"
	"github.com/vvb13a/goaudit/tui"
)

func main() {
	// 1. Config Manager
	cfgManager := service.NewManager("./data/config.json")

	// 2. Storage & Auto-migrations (GORM)
	db, err := store.Open("./data/goaudit.db")
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("failed to access sql db: %v", err)
	}
	defer sqlDB.Close()

	// 3. Engine & Registry
	// The app-level config acts as the fallback base for audit runs. Each audit
	// may carry its own config (stored on the audit record), which the runner
	// applies per run.
	allChecks := checks.All()
	registry := service.NewCheckRegistry(allChecks...)

	graphService := service.NewGraphService(db)
	linkValidator := service.NewLinkValidator(db)
	runner := service.NewRunner(graphService)
	notifier := service.NewNotifier()

	// 4. Application Services (GORM persistence)
	auditService := service.NewAuditService(db, graphService, linkValidator)

	// 5. Launch the selected interface: `goaudit serve` starts the web API
	// for the Vue frontend, anything else launches the TUI.
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		runServer(api.Deps{
			AuditService:  auditService,
			Registry:      registry,
			ConfigManager: cfgManager,
			Runner:        runner,
			Notifier:      notifier,
		})
		return
	}

	app := tui.New(tui.Deps{
		ConfigManager: cfgManager,
		Registry:      registry,
		Runner:        runner,
		AuditService:  auditService,
		Notifier:      notifier,
	})

	program := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

// runServer starts the JSON API that backs the web frontend. The listen
// address defaults to :8080 and can be overridden with GOAUDIT_ADDR.
func runServer(deps api.Deps) {
	addr := ":8080"
	if v := os.Getenv("GOAUDIT_ADDR"); v != "" {
		addr = v
	}
	log.Printf("goaudit api listening on %s", addr)
	if err := http.ListenAndServe(addr, api.NewServer(deps)); err != nil {
		log.Fatalf("api server: %v", err)
	}
}

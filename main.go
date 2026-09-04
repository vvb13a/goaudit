package main

import (
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vvb13a/goaudit/checks"
	"github.com/vvb13a/goaudit/service"
	"github.com/vvb13a/goaudit/store"
	"github.com/vvb13a/goaudit/tui"
)

func main() {
	// 1. Config Manager
	cfgManager := service.NewManager("./data/config.json")
	cfg := cfgManager.Get()

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
	// The app-level config acts as the fallback base for audit runs and sizes
	// the shared link cache. Each audit may carry its own config (stored on
	// the audit record), which the runner applies per run.
	linkCache := service.NewLinkCache(cfg.LinkCacheTTL())
	allChecks := checks.All(linkCache)
	registry := service.NewCheckRegistry(allChecks...)

	runner := service.NewRunner()

	// 4. Application Services (GORM persistence)
	excelService := service.NewExcelService("data/excel")
	htmlService := service.NewHtmlService("data/html")
	auditService := service.NewAuditService(db, excelService, htmlService)

	// 5. Launch TUI
	app := tui.New(tui.Deps{
		ConfigManager: cfgManager,
		Registry:      registry,
		Runner:        runner,
		AuditService:  auditService,
		ExcelService:  excelService,
		HtmlService:   htmlService,
	})

	program := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

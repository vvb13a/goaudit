package main

import (
	"fmt"
	"log"
	"os"

	"github.com/vvb13a/goaudit/checks"
	"github.com/vvb13a/goaudit/config"
	"github.com/vvb13a/goaudit/data"
	"github.com/vvb13a/goaudit/db"
	"github.com/vvb13a/goaudit/engine"
	"github.com/vvb13a/goaudit/internal/display/tui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	cfgManager := config.NewManager("./storage/config.json")
	cfg := cfgManager.Get()

	database, err := db.Open("./storage/goaudit.db")
	if err != nil {
		log.Fatalf("Failed to open SQLite database: %v", err)
	}
	defer database.Close()

	linkCache := engine.NewLinkCache(cfg.LinkCacheTTL())
	fetcher := engine.NewFetcher().
		WithTimeout(cfg.HTTPTimeout()).
		WithUserAgent(cfg.UserAgent).
		WithHeader("X-Inspection-Engine", "true")

	allChecks := []data.Check{
		checks.NewStatusCodeCheck(),
		checks.NewCanonicalURLCheck(),
		checks.NewDomSizeCheck(),
		checks.NewDocumentSizeCheck(),
		checks.NewEnforceHTTPSCheck(),
		checks.NewH1Check(),
		checks.NewHeadingHierarchyCheck(),
		checks.NewMixedContentCheck(),
		checks.NewHreflangCheck(),
		checks.NewImageIntegrityCheck(),
		checks.NewMetaDescriptionCheck(),
		checks.NewOpenGraphCheck(),
		checks.NewTwitterCardCheck(),
		checks.NewViewportCheck(),
		checks.NewTitleCheck(),
		checks.NewRobotsMetaCheck(),
		checks.NewTransferStatsLogCheck(),
		checks.NewPerformanceTimingsCheck(),
		checks.NewInternalLinksCheck(linkCache),
		checks.NewExternalLinksCheck(linkCache),
		checks.NewSchemaCheck(),
	}

	checklists, _ := database.ListChecklists()
	if len(checklists) == 0 {
		var checkNames []string
		for _, c := range allChecks {
			checkNames = append(checkNames, c.Name())
		}
		_ = database.SaveChecklist(&data.Checklist{
			Name:       "Full Audit (All Checks)",
			CheckNames: checkNames,
			IsDefault:  true,
		})
		checklists, _ = database.ListChecklists()
	}

	plans, _ := database.ListPlans()
	inspections, _ := database.ListInspections()

	app := tui.New(database, fetcher, cfg, allChecks, inspections, checklists, plans)
	program := tea.NewProgram(app, tea.WithAltScreen())

	if _, err := program.Run(); err != nil {
		fmt.Printf("Error starting TUI: %v\n", err)
		os.Exit(1)
	}
}

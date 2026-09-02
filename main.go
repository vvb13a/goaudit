package main

import (
	"context"
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vvb13a/goaudit/checks"
	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/service"
	"github.com/vvb13a/goaudit/store"
	"github.com/vvb13a/goaudit/tui"
)

func main() {
	ctx := context.Background()

	// 1. Config Manager
	cfgManager := service.NewManager("./data/config.json")
	cfg := cfgManager.Get()

	// 2. Storage & Auto-migrations (Goose)
	storeConn, err := store.Open("./data/goaudit.db")
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer storeConn.Close()

	// 3. Repositories
	planRepo := store.NewSQLPlanStore(storeConn.Queries)
	checklistRepo := store.NewSQLChecklistStore(storeConn.DB, storeConn.Queries)
	auditRepo := store.NewSQLAuditStore(storeConn.DB, storeConn.Queries)

	// 4. Engine & Registry
	linkCache := service.NewLinkCache(cfg.LinkCacheTTL())
	allChecks := checks.All(linkCache)
	registry := service.NewCheckRegistry(allChecks...)

	fetcher := service.NewFetcher().
		WithTimeout(cfg.HTTPTimeout()).
		WithUserAgent(cfg.UserAgent).
		WithHeader("X-Audit-Engine", "true")

	runner := service.NewRunner(fetcher, service.RunnerConfig{
		Concurrency:     cfg.MaxConcurrency,
		RequestDelay:    cfg.RequestDelay(),
		MaxSitemapDepth: cfg.MaxSitemapDepth,
	})

	// 5. Application Services
	planService := service.NewPlanService(planRepo)
	checklistService := service.NewChecklistService(checklistRepo, registry)
	auditService := service.NewAuditService(planRepo, checklistRepo, auditRepo, registry, runner)

	// 6. Seed Default Active Checklist if first run
	seedInitialChecklist(ctx, checklistService, registry)

	// 7. Launch TUI
	app := tui.New(
		planService,
		checklistService,
		auditService,
		cfgManager,
		registry,
	)

	program := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

func seedInitialChecklist(ctx context.Context, s *service.ChecklistService, r *service.CheckRegistry) {
	checklists, err := s.List(ctx)
	if err != nil || len(checklists) > 0 {
		return
	}

	allChecks := r.All()
	names := make([]string, 0, len(allChecks))
	for _, c := range allChecks {
		names = append(names, c.Info().Name)
	}

	initial := &domain.Checklist{
		Name:        "Full Audit (All Checks)",
		Description: "Runs all registered SEO, security, and performance rules.",
		CheckNames:  names,
		IsActive:    true,
	}

	_ = s.Create(ctx, initial)
}

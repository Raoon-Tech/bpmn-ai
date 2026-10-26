package bootstrap

import (
	"context"
	"fmt"
	"os"
	"strings"

	"gorm.io/gorm"

	"github.com/Raoon-Tech/bpmn-ai/config"
	"github.com/Raoon-Tech/bpmn-ai/internal/element/activities"
	"github.com/Raoon-Tech/bpmn-ai/internal/element/events"
	"github.com/Raoon-Tech/bpmn-ai/internal/element/flows"
	"github.com/Raoon-Tech/bpmn-ai/internal/element/gateways"
	"github.com/Raoon-Tech/bpmn-ai/internal/engine"
	"github.com/Raoon-Tech/bpmn-ai/internal/observability"
	"github.com/Raoon-Tech/bpmn-ai/internal/queue"
	"github.com/Raoon-Tech/bpmn-ai/pkg/ai"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/migrate"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/orm"
)

// Runtime is a fully wired engine runtime shared by `serve` and the business
// CLI commands.
type Runtime struct {
	Config     config.Config
	Logger     *observability.Logger
	Store      store.Store
	DB         *gorm.DB
	Engine     engine.Engine
	Queue      *queue.WorkerPool
	Registry   *engine.ElementRegistry
	Dispatcher *observability.Dispatcher

	closeFn func() error
}

// NewRuntime opens the store, builds the registry and wires the engine.
func NewRuntime(ctx context.Context, cfg config.Config, logger *observability.Logger) (*Runtime, error) {
	backend, err := Open(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	rt := &Runtime{
		Config: cfg,
		Logger: logger,
		Store:  backend.Store,
		DB:     backend.DB,
	}

	// Apply pending migrations when enabled (keeps CLI commands and serve in sync).
	if backend.DB != nil && cfg.Database.AutoMigrate {
		if _, err := migrate.Up(ctx, backend.DB, migrate.Options{}); err != nil {
			_ = backend.Close()
			return nil, fmt.Errorf("migrations: %w", err)
		}
	}

	dispatcher := observability.NewDispatcher()
	rt.Dispatcher = dispatcher

	if cfg.Webhook.Enabled && len(cfg.Webhook.URLs) > 0 {
		wh := observability.NewWebhookDispatcher(cfg.Webhook)
		dispatcher.Register("*", wh.Handle)
		logger.Info("webhooks enabled", "urls", cfg.Webhook.URLs, "events", cfg.Webhook.Events)
	}

	auditWriter, err := observability.NewFileAuditWriter(cfg.Audit.Dir, cfg.Audit.Enabled, logger)
	if err != nil {
		_ = backend.Close()
		return nil, fmt.Errorf("audit writer: %w", err)
	}
	_ = observability.NewAuditor(dispatcher, auditWriter)

	retry := queue.DefaultRetryPolicy()
	dlq := queue.NewDeadLetterQueue(rt.Store, rt.Store).WithDispatcher(dispatcher)
	q := queue.NewWorkerPool(rt.Store, nil, retry, dlq, queue.WorkerPoolConfig{
		Concurrency:  cfg.Engine.WorkerCount,
		PollInterval: cfg.Engine.QueuePollInterval,
	}).WithDispatcher(dispatcher)
	rt.Queue = q

	registry := engine.NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeTerminateEvent, events.NewTerminateEvent)
	registry.Register(bpmn.ElementTypeTimerEvent, events.NewTimerEvent)
	registry.Register(bpmn.ElementTypeMessageThrow, events.NewMessageThrowEvent)
	registry.Register(bpmn.ElementTypeMessageCatch, events.NewMessageCatchEvent)
	registry.Register(bpmn.ElementTypeExclusiveGateway, gateways.NewExclusiveGateway)
	registry.Register(bpmn.ElementTypeParallelGateway, gateways.NewParallelGateway)
	registry.Register(bpmn.ElementTypeInclusiveGateway, gateways.NewInclusiveGateway)
	registry.Register(bpmn.ElementTypeEventBasedGateway, gateways.NewEventBasedGateway)
	registry.Register(bpmn.ElementTypeUserTask, activities.NewUserTask)
	registry.Register(bpmn.ElementTypeScriptTask, activities.NewScriptTask)
	registry.Register(bpmn.ElementTypeServiceTask, activities.NewServiceTask)
	registry.Register(bpmn.ElementTypeSequenceFlow, flows.NewSequenceFlow)
	registry.Register(bpmn.ElementTypeSubProcess, activities.NewSubProcess)
	registry.Register(bpmn.ElementTypeErrorCatch, events.NewErrorCatchEvent)
	registry.Register(bpmn.ElementTypeErrorEnd, events.NewErrorEndEvent)
	registry.Register(bpmn.ElementTypeCallActivity, activities.NewCallActivity)
	registry.Register(bpmn.ElementTypeSignalThrow, events.NewSignalThrowEvent)
	registry.Register(bpmn.ElementTypeSignalCatch, events.NewSignalCatchEvent)
	rt.Registry = registry

	// AI wiring.
	aiRegistry := ai.NewProviderRegistry()
	aiRegistry.Register("openai", ai.NewOpenAIProvider)
	aiRegistry.Register("anthropic", ai.NewAnthropicProvider)

	providerPool := ai.NewProviderPool()
	toolRegistry := ai.NewToolRegistry()
	promptManager := ai.NewPromptManager()

	var aiGateway ai.Gateway
	var modelRouter *ai.ModelRouter
	var ragSystem *ai.RAGSystem

	openRouterKey := os.Getenv("AI_OPENROUTER_API_KEY")
	if openRouterKey != "" {
		orGW, err := aiRegistry.Create("openai", openRouterKey, "https://openrouter.ai/api/v1")
		if err != nil {
			logger.Error("failed to create OpenRouter provider", "error", err)
		} else {
			providerPool.Add("openrouter", orGW)
			orFallback, err := aiRegistry.Create("openai", openRouterKey, "https://openrouter.ai/api/v1")
			if err != nil {
				aiGateway = orGW
			} else {
				providerPool.Add("openrouter-fallback", orFallback)
				aiGateway = ai.NewFallbackGateway(orGW, orFallback)
			}
			logger.Info("OpenRouter configured as primary AI provider (200+ models)")
		}
	}

	if openRouterKey == "" && cfg.AI.APIKey != "" {
		primary, err := aiRegistry.Create(cfg.AI.Provider, cfg.AI.APIKey, cfg.AI.BaseURL)
		if err != nil {
			logger.Error("failed to create primary AI provider", "error", err)
		} else {
			providerPool.Add("primary", primary)
			providerPool.Add(cfg.AI.Provider, primary)
			secondary, err := aiRegistry.Create(cfg.AI.Provider, cfg.AI.APIKey, cfg.AI.BaseURL)
			if err != nil {
				aiGateway = primary
			} else {
				providerPool.Add("fallback", secondary)
				aiGateway = ai.NewFallbackGateway(primary, secondary)
			}
		}
	}

	if anthropicKey := os.Getenv("AI_ANTHROPIC_API_KEY"); anthropicKey != "" {
		if anthropicGW, err := aiRegistry.Create("anthropic", anthropicKey, ""); err == nil {
			providerPool.Add("anthropic", anthropicGW)
			logger.Info("Anthropic provider added to model router")
		}
	}
	if groqKey := os.Getenv("AI_GROQ_API_KEY"); groqKey != "" {
		if groqGW, err := aiRegistry.Create("openai", groqKey, "https://api.groq.com/openai/v1"); err == nil {
			providerPool.Add("groq", groqGW)
			logger.Info("Groq provider added to model router")
		}
	}
	if extra := os.Getenv("AI_EXTRA_PROVIDERS"); extra != "" {
		for _, entry := range strings.Split(extra, ",") {
			parts := strings.Split(strings.TrimSpace(entry), ":")
			if len(parts) >= 3 {
				if extraGW, err := aiRegistry.Create("openai", parts[1], parts[2]); err == nil {
					providerPool.Add(parts[0], extraGW)
					logger.Info("Extra provider added", "name", parts[0])
				}
			}
		}
	}

	// Load providers stored in the database (multi-provider configured via CLI).
	if rt.DB != nil {
		ps := orm.NewProviderStore(rt.DB)
		stored, err := ps.List(ctx)
		if err != nil {
			logger.Info("could not load AI providers from database", "error", err)
		} else {
			for _, p := range stored {
				gw, err := aiRegistry.Create(p.Type, p.APIKey, p.BaseURL)
				if err != nil {
					logger.Info("skip stored provider", "name", p.Name, "error", err)
					continue
				}
				if _, dup := providerPool.Get(p.Name); !dup {
					providerPool.Add(p.Name, gw)
				}
				if p.IsDefault && aiGateway == nil {
					aiGateway = gw
				}
				logger.Info("AI provider loaded from database", "name", p.Name, "type", p.Type)
			}
		}
	}

	if aiGateway != nil {
		modelRouter = ai.NewModelRouter(providerPool)

		profileModel := cfg.AI.DefaultModel
		if openRouterKey != "" {
			profileModel = "openrouter/openai/gpt-4o"
			modelRouter.SetProfiles(map[string]ai.Profile{
				"complex": {Model: "openrouter/openai/gpt-4o", MaxTokens: 8192, Priority: "quality"},
				"fast":    {Model: "openrouter/openai/gpt-4o-mini", MaxTokens: 2048, Priority: "speed"},
				"cheap":   {Model: "openrouter/openai/gpt-4o-mini", MaxTokens: 1024, Priority: "cost"},
				"auto":    {Model: "openrouter/openai/gpt-4o", MaxTokens: 4096, Priority: "quality"},
			})
		}
		modelRouter.AddProfile("auto", ai.Profile{Model: profileModel, MaxTokens: cfg.AI.MaxTokens, Priority: "quality"})

		// Apply model profiles stored in the database (overrides built-in defaults).
		if rt.DB != nil {
			pstore := orm.NewProfileStore(rt.DB)
			stored, err := pstore.List(ctx)
			if err != nil {
				logger.Info("could not load model profiles from database", "error", err)
			} else {
				for _, p := range stored {
					modelRouter.AddProfile(p.Name, ai.Profile{
						Model:     p.Model,
						MaxTokens: p.MaxTokens,
						Priority:  p.Priority,
					})
					logger.Info("model profile loaded from database", "name", p.Name, "model", p.Model)
				}
			}
		}

		embedderKey := cfg.AI.APIKey
		embedderBase := cfg.AI.BaseURL
		if openRouterKey != "" {
			embedderKey = openRouterKey
			embedderBase = "https://openrouter.ai/api/v1"
		}
		embedder, err := ai.NewOpenAIEmbedder(embedderKey, embedderBase)
		if err == nil {
			ragSystem = ai.NewRAGSystem(embedder)
			ragSystem.AddCollection("default", ai.NewInMemoryVectorStore())
			logger.Info("RAG system initialized with default collection")
		} else {
			logger.Info("failed to create embedder, RAG disabled", "error", err)
		}

		registry.Register(bpmn.ElementTypeAITask, activities.NewAITaskConstructor(aiGateway, toolRegistry, modelRouter, ragSystem, promptManager))
		logger.Info("AI task element registered with model router",
			"providers", providerPool.List(),
			"default_model", cfg.AI.DefaultModel,
			"default_profile", cfg.AI.DefaultProfile,
		)
	} else {
		logger.Info("AI not configured — set AI_API_KEY or use 'bpmn-ai ai provider add'")
	}

	eng := engine.New(engine.Config{
		WorkerCount:      cfg.Engine.WorkerCount,
		MaxLoops:         cfg.Engine.MaxLoops,
		ExecutionTimeout: cfg.Engine.ExecutionTimeout,
	}, registry, rt.Store, logger, q)
	eng.WithDispatcher(dispatcher)
	rt.Engine = eng

	q.WithHandler(eng.JobHandler())

	rt.closeFn = func() error {
		q.Stop()
		_ = auditWriter.Close()
		return backend.Close()
	}

	return rt, nil
}

// Start begins the job queue workers.
func (rt *Runtime) Start(ctx context.Context) {
	rt.Queue.Start(ctx)
}

// Stop signals the job queue workers to shut down.
func (rt *Runtime) Stop() {
	rt.Queue.Stop()
}

// Close releases all resources held by the runtime.
func (rt *Runtime) Close() error {
	if rt.closeFn != nil {
		return rt.closeFn()
	}
	return nil
}

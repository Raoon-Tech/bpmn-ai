package http

import (
	"github.com/go-chi/chi/v5"
	"github.com/Raoon-Tech/bpmn-ai/api/middleware"
	"github.com/Raoon-Tech/bpmn-ai/internal/observability"
)

func (s *Server) routes() {
	r := s.router

	r.Get("/health", s.healthCheck)
	r.Get("/ready", s.readinessCheck)

	if s.metrics != nil {
		r.Handle("/metrics", observability.DefaultMetrics().Handler())
	}

	r.Route("/api/v1", func(r chi.Router) {
		if !s.config.DisableCSRF {
			r.Use(middleware.CSRF)
		}
		if s.config.Auth.Mode != "" && s.config.Auth.Mode != "none" {
			r.Use(middleware.Auth(s.config.Auth))
		}

		r.Get("/csrf-token", s.getCSRFToken)
		r.Post("/processes", s.createProcess)
		r.Get("/processes", s.listProcesses)
		r.Get("/processes/{id}", s.getProcess)
		r.Post("/processes/{id}/start", s.startCase)

		r.Get("/cases", s.listCases)
		r.Get("/cases/{id}", s.getCase)
		r.Get("/cases/{id}/tasks", s.getCaseTasks)
		r.Get("/cases/{id}/history", s.getCaseHistory)
		r.Get("/cases/{id}/diagram", s.getCaseDiagram)

		r.Post("/tasks/{id}/claim", s.claimTask)
		r.Post("/tasks/{id}/complete", s.completeTask)

		r.Post("/messages", s.sendMessage)
		r.Post("/signals", s.sendSignal)
	})
}

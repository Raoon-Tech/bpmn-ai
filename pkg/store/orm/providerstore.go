package orm

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ProviderConfig is a configured AI provider.
type ProviderConfig struct {
	Name         string
	Type         string // openai, anthropic
	BaseURL      string
	APIKey       string
	DefaultModel string
	IsDefault    bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ProviderStore manages AI provider configurations in the database.
type ProviderStore struct {
	db *gorm.DB
}

// NewProviderStore wraps an open GORM handle.
func NewProviderStore(db *gorm.DB) *ProviderStore {
	return &ProviderStore{db: db}
}

// List returns all configured providers.
func (s *ProviderStore) List(ctx context.Context) ([]ProviderConfig, error) {
	var models []AIProviderModel
	if err := s.db.WithContext(ctx).Order("name").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]ProviderConfig, 0, len(models))
	for i := range models {
		out = append(out, fromAIProviderModel(&models[i]))
	}
	return out, nil
}

// Get returns a single provider by name.
func (s *ProviderStore) Get(ctx context.Context, name string) (ProviderConfig, error) {
	var m AIProviderModel
	err := s.db.WithContext(ctx).Where("name = ?", name).First(&m).Error
	if err != nil {
		return ProviderConfig{}, err
	}
	return fromAIProviderModel(&m), nil
}

// Save creates or updates a provider.
func (s *ProviderStore) Save(ctx context.Context, cfg ProviderConfig) error {
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = time.Now()
	}
	cfg.UpdatedAt = time.Now()
	m := toAIProviderModel(cfg)
	return s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{"type", "base_url", "api_key", "default_model", "is_default", "updated_at"}),
		}).
		Create(m).Error
}

// Delete removes a provider by name.
func (s *ProviderStore) Delete(ctx context.Context, name string) error {
	res := s.db.WithContext(ctx).Where("name = ?", name).Delete(&AIProviderModel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func toAIProviderModel(cfg ProviderConfig) *AIProviderModel {
	return &AIProviderModel{
		Name:         cfg.Name,
		Type:         cfg.Type,
		BaseURL:      cfg.BaseURL,
		APIKey:       cfg.APIKey,
		DefaultModel: cfg.DefaultModel,
		IsDefault:    cfg.IsDefault,
		CreatedAt:    cfg.CreatedAt,
		UpdatedAt:    cfg.UpdatedAt,
	}
}

func fromAIProviderModel(m *AIProviderModel) ProviderConfig {
	return ProviderConfig{
		Name:         m.Name,
		Type:         m.Type,
		BaseURL:      m.BaseURL,
		APIKey:       m.APIKey,
		DefaultModel: m.DefaultModel,
		IsDefault:    m.IsDefault,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

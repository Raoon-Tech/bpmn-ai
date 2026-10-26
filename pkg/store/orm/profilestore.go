package orm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ModelProfile is a configured model routing profile.
type ModelProfile struct {
	Name      string
	Model     string
	MaxTokens int
	Priority  string // quality, speed, cost
	UpdatedAt time.Time
}

// ProfileStore manages AI model profiles in the database.
type ProfileStore struct {
	db *gorm.DB
}

// NewProfileStore wraps an open GORM handle.
func NewProfileStore(db *gorm.DB) *ProfileStore {
	return &ProfileStore{db: db}
}

// validProfileNames is the fixed set of profile names.
var validProfileNames = map[string]bool{
	"complex": true,
	"fast":    true,
	"cheap":   true,
	"auto":    true,
}

// IsValidProfileName reports whether name is one of the supported profiles.
func IsValidProfileName(name string) bool {
	return validProfileNames[name]
}

// ValidProfileNames returns the supported profile names in stable order.
func ValidProfileNames() []string {
	return []string{"complex", "fast", "cheap", "auto"}
}

// List returns all configured profiles.
func (s *ProfileStore) List(ctx context.Context) ([]ModelProfile, error) {
	var models []AIModelProfile
	if err := s.db.WithContext(ctx).Order("name").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]ModelProfile, 0, len(models))
	for i := range models {
		out = append(out, fromAIModelProfile(&models[i]))
	}
	return out, nil
}

// Get returns a single profile by name.
func (s *ProfileStore) Get(ctx context.Context, name string) (ModelProfile, error) {
	var m AIModelProfile
	err := s.db.WithContext(ctx).Where("name = ?", name).First(&m).Error
	if err != nil {
		return ModelProfile{}, err
	}
	return fromAIModelProfile(&m), nil
}

// Save creates or updates a profile.
func (s *ProfileStore) Save(ctx context.Context, p ModelProfile) error {
	if !IsValidProfileName(p.Name) {
		return fmt.Errorf("unknown profile %q (expected one of: %s)", p.Name, joinNames(ValidProfileNames()))
	}
	if p.MaxTokens <= 0 {
		p.MaxTokens = 4096
	}
	if p.Priority == "" {
		p.Priority = "quality"
	}
	p.UpdatedAt = time.Now()
	m := toAIModelProfile(p)
	return s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{"model", "max_tokens", "priority", "updated_at"}),
		}).
		Create(m).Error
}

// Delete removes a profile by name.
func (s *ProfileStore) Delete(ctx context.Context, name string) error {
	res := s.db.WithContext(ctx).Where("name = ?", name).Delete(&AIModelProfile{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func toAIModelProfile(p ModelProfile) *AIModelProfile {
	return &AIModelProfile{
		Name:      p.Name,
		Model:     p.Model,
		MaxTokens: p.MaxTokens,
		Priority:  p.Priority,
		UpdatedAt: p.UpdatedAt,
	}
}

func fromAIModelProfile(m *AIModelProfile) ModelProfile {
	return ModelProfile{
		Name:      m.Name,
		Model:     m.Model,
		MaxTokens: m.MaxTokens,
		Priority:  m.Priority,
		UpdatedAt: m.UpdatedAt,
	}
}

func joinNames(names []string) string {
	return strings.Join(names, ", ")
}

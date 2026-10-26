package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// Scaffold creates a new migration file in dir and returns its path.
func Scaffold(dir, name string) (string, error) {
	slug := slugify(name)
	if slug == "" {
		return "", fmt.Errorf("migration name must contain at least one letter or digit")
	}
	if strings.TrimSpace(dir) == "" {
		dir = "pkg/store/migrations"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create migrations directory: %w", err)
	}

	base := time.Now()
	for i := 0; i < 120; i++ {
		version := base.Add(time.Duration(i) * time.Second).Format("20060102150405")
		path := filepath.Join(dir, version+"_"+slug+".go")
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("create migration file: %w", err)
		}
		_, writeErr := fmt.Fprintf(file, migrationTemplate, version, version, version)
		closeErr := file.Close()
		if writeErr != nil {
			return "", fmt.Errorf("write migration file: %w", writeErr)
		}
		if closeErr != nil {
			return "", fmt.Errorf("close migration file: %w", closeErr)
		}
		return path, nil
	}
	return "", fmt.Errorf("could not allocate a unique migration version in %s", dir)
}

func slugify(name string) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastUnderscore = false
		case !lastUnderscore:
			b.WriteRune('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

const migrationTemplate = `package migrations

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/Raoon-Tech/bpmn-ai/pkg/store/migrate"
)

func init() {
	migrate.Register(migrate.Migration{
		Version: "%s",
		Up: func(tx *gorm.DB) error {
			// TODO: implement the forward migration.
			return fmt.Errorf("migration %s is not implemented yet")
		},
		Down: func(tx *gorm.DB) error {
			// TODO: implement the rollback, or return migrate.ErrIrreversible.
			return fmt.Errorf("migration %s rollback is not implemented yet")
		},
	})
}
`

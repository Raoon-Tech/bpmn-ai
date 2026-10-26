package migrations

import (
	"gorm.io/gorm"

	"github.com/Raoon-Tech/bpmn-ai/pkg/store/migrate"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/orm"
)

// aiModelProfiles adds the ai_model_profiles table used to store model routing
// profiles (complex, fast, cheap, auto) configured through the CLI.
func init() {
	migrate.Register(migrate.Migration{
		Version: "0003_ai_model_profiles",
		Up:      func(tx *gorm.DB) error { return orm.AutoMigrate(tx) },
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("ai_model_profiles")
		},
	})
}

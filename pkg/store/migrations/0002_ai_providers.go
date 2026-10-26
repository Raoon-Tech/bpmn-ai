package migrations

import (
	"gorm.io/gorm"

	"github.com/Raoon-Tech/bpmn-ai/pkg/store/migrate"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/orm"
)

// aiProviders adds the ai_providers table used to store configured AI
// providers (multi-provider setup) managed through the CLI.
func init() {
	migrate.Register(migrate.Migration{
		Version: "0002_ai_providers",
		Up:      func(tx *gorm.DB) error { return orm.AutoMigrate(tx) },
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("ai_providers")
		},
	})
}

package migrations

import (
	"gorm.io/gorm"

	"github.com/Raoon-Tech/bpmn-ai/pkg/store/migrate"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/orm"
)

// initialSchema creates every table managed by the ORM store. It is the only
// migration allowed to rely on AutoMigrate: later schema changes must be
// explicit, versioned migrations.
func init() {
	migrate.Register(migrate.Migration{
		Version: "0001_initial_schema",
		Up:      func(tx *gorm.DB) error { return orm.AutoMigrate(tx) },
		Down:    func(tx *gorm.DB) error { return orm.DropAll(tx) },
	})
}

package migrations

import (
	"gorm.io/gorm"

	"github.com/Raoon-Tech/bpmn-ai/pkg/store/migrate"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/orm"
)

// tenantID adds the tenant_id column to instances for multi-tenant isolation.
func init() {
	migrate.Register(migrate.Migration{
		Version: "0004_tenant_id",
		Up:      func(tx *gorm.DB) error { return orm.AutoMigrate(tx) },
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropColumn(&orm.InstanceModel{}, "TenantID")
		},
	})
}

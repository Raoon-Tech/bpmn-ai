// Package orm implements the BPMN engine persistence contract (store.Store)
// on top of GORM.
//
// It supports PostgreSQL, MySQL and SQLite. The schema is described by the
// models in this package and can be created either through the versioned
// migration runner (package migrate) or directly with AutoMigrate.
package orm

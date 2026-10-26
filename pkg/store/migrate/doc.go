// Package migrate implements versioned schema migrations for the ORM store.
//
// Migrations are Go files that register themselves through Register. They are
// applied in ascending version order and tracked in the schema_migrations
// table. Each migration and its bookkeeping row run inside the same
// transaction.
package migrate

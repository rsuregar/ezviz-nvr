package db

import (
	"nvr-ezviz/api/internal/models"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Connect(dsn string) (*gorm.DB, error) {
	gdb, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}
	return gdb, nil
}

// Migrate applies schema changes via GORM AutoMigrate. This is fine for an
// early-stage project with one deployment target; swap for versioned SQL
// migrations (golang-migrate/atlas) once the schema needs review control.
func Migrate(gdb *gorm.DB) error {
	return gdb.AutoMigrate(models.AllModels()...)
}

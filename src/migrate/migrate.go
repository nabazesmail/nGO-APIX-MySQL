// migration.go
package migrate

import (
	"fmt"
	"log"
	"time"

	"gorm.io/gorm/logger"

	"github.com/nabazesmail/gopher/src/initializers"
	"github.com/nabazesmail/gopher/src/models"
)

func Migration() {
	// Load environment variables and connect to the database
	initializers.LoadEnvVariables()
	initializers.ConnectToDB()

	// Setting up a custom logger to control the verbosity of logs during migrations
	migrationLogger := logger.New(
		log.New(log.Writer(), "\r\n", log.LstdFlags), // Use the same log.Writer as the default logger
		logger.Config{
			SlowThreshold: time.Second, // Set the slow threshold for migrations (adjust as needed)
			LogLevel:      logger.Info, // Set the log level to Info to show migration logs
		},
	)

	//  a new gorm.DB instance with the custom logger
	migrator := initializers.DB.WithContext(initializers.DB.Statement.Context)
	migrator.Logger = migrationLogger

	// this Checks if the User table exists in the database
	if migrator.Migrator().HasTable(&models.User{}) {
		fmt.Println("Database schema is up to date. No migration needed.")
	} else {
		// this Runs the auto migration for the User model
		err := migrator.AutoMigrate(&models.User{})
		if err != nil {
			log.Fatalf("Failed to run auto migration: %v", err)
		}

		fmt.Println("Database schema updated successfully.")
	}
}

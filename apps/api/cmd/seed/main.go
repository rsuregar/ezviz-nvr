// Command seed creates the first superadmin account so someone can log in
// and start managing the system (no self-registration is exposed by the API).
package main

import (
	"flag"
	"log"

	"nvr-ezviz/api/internal/auth"
	"nvr-ezviz/api/internal/config"
	"nvr-ezviz/api/internal/db"
	"nvr-ezviz/api/internal/models"
)

func main() {
	email := flag.String("email", "admin@example.com", "superadmin email")
	password := flag.String("password", "", "superadmin password (required)")
	name := flag.String("name", "Administrator", "superadmin display name")
	flag.Parse()

	if *password == "" {
		log.Fatal("--password is required")
	}

	cfg := config.Load()
	gdb, err := db.Connect(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	if err := db.Migrate(gdb); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	hash, err := auth.HashPassword(*password)
	if err != nil {
		log.Fatalf("failed to hash password: %v", err)
	}

	user := models.User{
		Email:        *email,
		PasswordHash: hash,
		Name:         *name,
		IsSuperAdmin: true,
	}
	if err := gdb.Where("email = ?", *email).FirstOrCreate(&user).Error; err != nil {
		log.Fatalf("failed to create superadmin: %v", err)
	}

	log.Printf("superadmin ready: %s", *email)
}

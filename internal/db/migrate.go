package db

import (
	"fmt"

	"database/sql"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)


func RunMigrations(db *sql.DB, migrationsPath string ) error {
	driver , err:= postgres.WithInstance(db, &postgres.Config{})
	if err != nil{
		return fmt.Errorf("could not create friver for migration: %w",err)
	}
	m,err := migrate.NewWithDatabaseInstance(migrationsPath,"postgres",driver)
	if err != nil{
		return fmt.Errorf("could not crete migrator : %w",err)
	} 

	if err := m.Up(); err!=nil && err!=migrate.ErrNoChange{
		return fmt.Errorf("migration failed %w",err)
	}
	return nil


}


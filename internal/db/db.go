package db

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

func Connect(dsn string) (*sql.DB,error){
	db,err:= sql.Open("postgres",dsn)
	if err!= nil{
		return nil, fmt .Errorf("failed to open db: %w",err)
	}

	if err := db.Ping(); err!=nil{
		return nil,fmt.Errorf("failed to pingdb %w",err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)

	return db,nil
}
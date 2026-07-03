package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type User struct{
	ID string
	Email string
	Password string
	CreatedAt time.Time
}

func CreateUser(db *sql.DB, email,hashedPassword string) (string,error){
	var id string
	query := `INSERT INTO users (email,password) VALUES ($1,$2) RETURNING id`
	err := db.QueryRow(query,email,hashedPassword).Scan(&id)
	if err != nil{
		return "", fmt.Errorf("create user: %w",err)
	}

	return id,nil

}

func GetUserByEmail(email string, db *sql.DB) (*User,error){
	u := &User{}
	query:= `SELECT id, email, password, created_at FROM users WHERE email = $1`
	err:=db.QueryRow(query,email).Scan(&u.ID,&u.Email,&u.Password,&u.CreatedAt)
	if errors.Is(err,sql.ErrNoRows){return nil,nil} // user does not exissst

	if err != nil{
		return nil, fmt.Errorf("get user y email: %w",err)
	}
	return u, nil
}



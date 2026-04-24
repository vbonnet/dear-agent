//go:build ignore

package main

import (
	"database/sql"
	"net/http"
)

type AuthService struct {
	db *sql.DB
}

func (s *AuthService) HandleLogin(w http.ResponseWriter, r *http.Request) {
	// Authentication logic
}

func main() {
	db, _ := sql.Open("postgres", "...")
	service := &AuthService{db: db}
	http.HandleFunc("/login", service.HandleLogin)
	http.ListenAndServe(":8080", nil)
}

// resetpassword sets a user's password by email (local/self-host recovery; no email flow).
// Usage: DATABASE_URL=... go run ./cmd/resetpassword <email> <new-password>
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"hyperspeed/api/internal/auth"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: resetpassword <email> <new-password>")
		fmt.Fprintln(os.Stderr, "  Set DATABASE_URL (e.g. postgres://hyperspeed:hyperspeed@localhost:5432/hyperspeed?sslmode=disable)")
		os.Exit(2)
	}
	email := strings.TrimSpace(strings.ToLower(os.Args[1]))
	pass := os.Args[2]
	if len(pass) < 8 {
		fmt.Fprintln(os.Stderr, "password must be at least 8 characters")
		os.Exit(1)
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(1)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "db:", err)
		os.Exit(1)
	}
	defer pool.Close()

	hash, err := auth.HashPassword(pass)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hash:", err)
		os.Exit(1)
	}
	ct, err := pool.Exec(ctx, `UPDATE users SET password_hash = $1 WHERE LOWER(email) = $2`, hash, email)
	if err != nil {
		fmt.Fprintln(os.Stderr, "update:", err)
		os.Exit(1)
	}
	if ct.RowsAffected() == 0 {
		fmt.Fprintf(os.Stderr, "no user with email %q\n", email)
		os.Exit(1)
	}
	fmt.Println("password updated for", email)
}

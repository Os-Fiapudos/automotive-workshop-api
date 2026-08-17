package auth

// User is an administrative user (docs/entities.md "User").
type User struct {
	ID           string
	Code         int64
	Name         string
	Email        string
	PasswordHash string
}

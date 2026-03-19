package db

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// UserContextKey is the context key for storing user information.
type contextKey string

const UserContextKey contextKey = "user"

// User represents the authenticated user in context.
type User struct {
	ID   string
	Role string // "user" or "admin"
}

// SetUserContext sets the user in the context.
func SetUserContext(ctx context.Context, user *User) context.Context {
	return context.WithValue(ctx, UserContextKey, user)
}

// GetUserFromContext retrieves the user from the context.
func GetUserFromContext(ctx context.Context) (*User, bool) {
	user, ok := ctx.Value(UserContextKey).(*User)
	return user, ok
}

// AuthTx wraps a GORM transaction with PostgreSQL RLS claims.
// This is CRITICAL for multi-tenant security - all user-scoped database
// operations MUST use AuthTx to ensure Row-Level Security policies are enforced.
//
// Usage:
//   err := db.AuthTx(ctx, db.GetGorm(), func(tx *gorm.DB) error {
//       var item model.LibraryItem
//       return tx.First(&item, "id = ?", id).Error
//   })
//
// The user is extracted from ctx (set by auth middleware).
// If no user is in context, the transaction proceeds without claims
// (useful for system operations like migrations or background jobs).
func AuthTx(ctx context.Context, db *gorm.DB, fn func(tx *gorm.DB) error) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// Extract user from context
		user, hasUser := GetUserFromContext(ctx)
		
		if hasUser {
			// Set PostgreSQL RLS claims
			if err := SetClaims(tx, user.ID, user.Role); err != nil {
				return fmt.Errorf("failed to set claims: %w", err)
			}
		}
		
		// Execute the user's function within the transaction
		return fn(tx)
	})
}

// SetClaims calls the PostgreSQL omnivore.set_claims function to set
// session variables that RLS policies use to filter data.
//
// The claims are:
//   - omnivore_user.uid: The user's UUID
//   - role: Either 'omnivore_user' or 'omnivore_admin'
//
// These are set as LOCAL variables, so they only apply to the current transaction.
func SetClaims(tx *gorm.DB, userID, role string) error {
	dbRole := "omnivore_user"
	if role == "admin" {
		dbRole = "omnivore_admin"
	}
	
	// Call the PostgreSQL function
	result := tx.Exec("SELECT omnivore.set_claims($1, $2)", userID, dbRole)
	if result.Error != nil {
		return fmt.Errorf("omnivore.set_claims failed: %w", result.Error)
	}
	
	return nil
}

// AuthTxWithUser is a convenience wrapper that creates a context with the user
// and calls AuthTx. Useful when you have user info but no context with it yet.
func AuthTxWithUser(ctx context.Context, db *gorm.DB, userID, role string, fn func(tx *gorm.DB) error) error {
	ctx = SetUserContext(ctx, &User{ID: userID, Role: role})
	return AuthTx(ctx, db, fn)
}

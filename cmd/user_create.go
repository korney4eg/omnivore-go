package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/omnivore-app/omnivore/internal/config"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/service"
	"github.com/spf13/cobra"
)

var (
	createUserEmail    string
	createUserPassword string
	createUserName     string
	createUserUsername string
)

var userCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an email/password user",
	RunE:  runUserCreate,
}

func init() {
	userCmd.AddCommand(userCreateCmd)

	userCreateCmd.Flags().StringVar(&createUserEmail, "email", "", "Email address for the new user")
	userCreateCmd.Flags().StringVar(&createUserPassword, "password", "", "Password for the new user")
	userCreateCmd.Flags().StringVar(&createUserName, "name", "", "Display name for the new user (defaults to username)")
	userCreateCmd.Flags().StringVar(&createUserUsername, "username", "", "Profile username (defaults to email local part)")
	userCreateCmd.Flags().StringVar(&createUserUsername, "user-name", "", "Deprecated alias for --username")

	userCreateCmd.MarkFlagRequired("email")
	userCreateCmd.MarkFlagRequired("password")
	userCreateCmd.Flags().MarkDeprecated("user-name", "use --username instead")
}

func runUserCreate(cmd *cobra.Command, args []string) error {
	dsn := config.BuildDatabaseURL()
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL or PG_* variables required")
	}

	if _, err := db.Connect(dsn); err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer db.Close()

	userSvc := service.NewUserService(db.GetGorm())
	result, err := userSvc.CreateEmailUser(context.Background(), service.CreateEmailUserInput{
		Email:    createUserEmail,
		Password: createUserPassword,
		Name:     createUserName,
		Username: createUserUsername,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrUserAlreadyExists):
			return fmt.Errorf("user with email %s already exists", createUserEmail)
		case errors.Is(err, service.ErrUsernameTaken):
			return fmt.Errorf("username %q already exists", createUserUsername)
		default:
			return err
		}
	}

	fmt.Printf("✓ Created user %s (%s) with username %s\n", result.Email, result.UserID, result.Username)
	return nil
}

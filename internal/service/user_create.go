package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/omnivore-app/omnivore/internal/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrUserAlreadyExists = errors.New("user already exists")
	ErrUsernameTaken     = errors.New("username already exists")
)

// UserService handles user creation flows shared by HTTP handlers and CLI.
type UserService struct {
	db *gorm.DB
}

// CreateEmailUserInput is the input for creating a local email/password user.
type CreateEmailUserInput struct {
	Email    string
	Password string
	Name     string
	Username string
}

// CreateEmailUserResult is the result of creating a local email/password user.
type CreateEmailUserResult struct {
	UserID   uuid.UUID
	Email    string
	Name     string
	Username string
}

// NewUserService creates a new UserService.
func NewUserService(database *gorm.DB) *UserService {
	return &UserService{db: database}
}

// CreateEmailUser creates a new Omnivore EMAIL user and matching profile row.
func (s *UserService) CreateEmailUser(ctx context.Context, input CreateEmailUserInput) (*CreateEmailUserResult, error) {
	normalized, err := normalizeCreateEmailUserInput(input)
	if err != nil {
		return nil, err
	}

	var existingUser model.User
	err = s.db.WithContext(ctx).
		Where("email = ?", normalized.Email).
		First(&existingUser).Error
	if err == nil {
		return nil, ErrUserAlreadyExists
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("check existing user: %w", err)
	}

	var existingProfile model.Profile
	err = s.db.WithContext(ctx).
		Where("username = ?", normalized.Username).
		First(&existingProfile).Error
	if err == nil {
		return nil, ErrUsernameTaken
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("check existing username: %w", err)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(normalized.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	userID := uuid.New()
	hashedPwd := string(hashedPassword)

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			`INSERT INTO omnivore."user" (id, source, email, source_user_id, name, password)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			userID,
			model.RegistrationEmail,
			normalized.Email,
			normalized.Email,
			normalized.Name,
			hashedPwd,
		).Error; err != nil {
			return fmt.Errorf("create user: %w", err)
		}

		if err := tx.Exec(
			`INSERT INTO omnivore.user_profile (user_id, username) VALUES (?, ?)`,
			userID,
			normalized.Username,
		).Error; err != nil {
			return fmt.Errorf("create profile: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &CreateEmailUserResult{
		UserID:   userID,
		Email:    normalized.Email,
		Name:     normalized.Name,
		Username: normalized.Username,
	}, nil
}

func normalizeCreateEmailUserInput(input CreateEmailUserInput) (CreateEmailUserInput, error) {
	normalized := CreateEmailUserInput{
		Email:    strings.TrimSpace(input.Email),
		Password: input.Password,
		Name:     strings.TrimSpace(input.Name),
		Username: normalizeUsername(input.Username),
	}

	if normalized.Email == "" {
		return CreateEmailUserInput{}, fmt.Errorf("email is required")
	}
	if len(normalized.Password) < 8 {
		return CreateEmailUserInput{}, fmt.Errorf("password must be at least 8 characters")
	}

	if normalized.Username == "" {
		normalized.Username = defaultUsernameFromEmail(normalized.Email)
	}
	if normalized.Username == "" {
		return CreateEmailUserInput{}, fmt.Errorf("username is required")
	}

	if normalized.Name == "" {
		normalized.Name = normalized.Username
	}

	return normalized, nil
}

func defaultUsernameFromEmail(email string) string {
	localPart := email
	if at := strings.Index(localPart, "@"); at >= 0 {
		localPart = localPart[:at]
	}
	return normalizeUsername(localPart)
}

func normalizeUsername(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '_':
			b.WriteRune(r)
		}
	}

	return b.String()
}

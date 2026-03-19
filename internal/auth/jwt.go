package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// JWTConfig holds JWT configuration.
type JWTConfig struct {
	PrivateKey *rsa.PrivateKey
	PublicKey  *rsa.PublicKey
	Issuer     string
	Expiration time.Duration
}

// Claims represents JWT claims.
type Claims struct {
	UserID   string `json:"uid"`
	UserRole string `json:"userRole"`
	jwt.RegisteredClaims
}

// DefaultJWTConfig creates a JWT config with defaults.
// If JWT_SECRET env is set, uses it; otherwise generates a new key pair.
func DefaultJWTConfig() (*JWTConfig, error) {
	// Try to load from environment
	if jwtSecret := os.Getenv("JWT_SECRET"); jwtSecret != "" {
		privateKey, err := parsePrivateKey([]byte(jwtSecret))
		if err != nil {
			return nil, fmt.Errorf("failed to parse JWT_SECRET: %w", err)
		}
		
		return &JWTConfig{
			PrivateKey: privateKey,
			PublicKey:  &privateKey.PublicKey,
			Issuer:     "omnivore-api",
			Expiration: 7 * 24 * time.Hour, // 7 days
		}, nil
	}

	// Generate new key pair (development only)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	return &JWTConfig{
		PrivateKey: privateKey,
		PublicKey:  &privateKey.PublicKey,
		Issuer:     "omnivore-api",
		Expiration: 7 * 24 * time.Hour,
	}, nil
}

// GenerateToken creates a JWT token for a user.
func (c *JWTConfig) GenerateToken(userID uuid.UUID, userRole string) (string, error) {
	now := time.Now()
	
	claims := Claims{
		UserID:   userID.String(),
		UserRole: userRole,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    c.Issuer,
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(c.Expiration)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	
	signedToken, err := token.SignedString(c.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return signedToken, nil
}

// ValidateToken validates a JWT token and returns the claims.
func (c *JWTConfig) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return c.PublicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, fmt.Errorf("invalid claims type")
	}

	return claims, nil
}

// parsePrivateKey parses an RSA private key from PEM format.
func parsePrivateKey(keyData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS8 format
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
		
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("not an RSA private key")
		}
		return rsaKey, nil
	}

	return privateKey, nil
}

// ExportPublicKeyPEM exports the public key as PEM format.
func (c *JWTConfig) ExportPublicKeyPEM() (string, error) {
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(c.PublicKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal public key: %w", err)
	}

	pubKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: pubKeyBytes,
	})

	return string(pubKeyPEM), nil
}

// RefreshToken generates a new token if the current one is close to expiry.
func (c *JWTConfig) RefreshToken(tokenString string) (string, error) {
	claims, err := c.ValidateToken(tokenString)
	if err != nil {
		return "", err
	}

	// Check if token is within 1 day of expiry
	expiresIn := time.Until(claims.ExpiresAt.Time)
	if expiresIn > 24*time.Hour {
		// Token still valid for more than a day, no need to refresh
		return tokenString, nil
	}

	// Generate new token
	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		return "", fmt.Errorf("invalid user ID in token: %w", err)
	}

	return c.GenerateToken(userID, claims.UserRole)
}

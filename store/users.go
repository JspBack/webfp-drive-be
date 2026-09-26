package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	authhelper "github.com/JspBack/webfp-drive/helper/auth"
	storehelper "github.com/JspBack/webfp-drive/helper/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrEmailTaken   = errors.New("email is already registered")
	ErrInvalidCreds = errors.New("invalid email or password")
)

var ErrInvalidSession = errors.New("invalid or expired session")

type User struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UserStore struct {
	db              *pgxpool.Pool
	cache           *redis.Client
	sessionLifetime time.Duration
}

func (us *UserStore) Authenticate(ctx context.Context, email, password string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	user := &User{}
	var passwordHash string
	err := us.db.QueryRow(ctx, `
		SELECT u.id, u.email, u.name, u.created_at, u.updated_at, a.password_hash
		FROM users u
		JOIN auth_identities a ON a.user_id = u.id
		WHERE a.provider = 'password' AND a.provider_subject = $1
	`, email).Scan(
		&user.ID, &user.Email, &user.Name, &user.CreatedAt, &user.UpdatedAt, &passwordHash,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidCreds
		}
		return nil, fmt.Errorf("find login identity: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCreds
	}
	return user, nil
}

func (us *UserStore) CreateSession(ctx context.Context, userID uuid.UUID) (string, error) {
	token, err := authhelper.NewSessionToken()
	if err != nil {
		return "", err
	}
	if err := us.cache.Set(ctx, authhelper.SessionKey(token), userID.String(), us.sessionLifetime).Err(); err != nil {
		return "", fmt.Errorf("save session: %w", err)
	}
	return token, nil
}

func (us *UserStore) ResolveSession(ctx context.Context, token string) (uuid.UUID, error) {
	value, err := us.cache.Get(ctx, authhelper.SessionKey(token)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return uuid.Nil, ErrInvalidSession
		}
		return uuid.Nil, fmt.Errorf("load session: %w", err)
	}
	userID, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse session user ID: %w", err)
	}
	return userID, nil
}

func (us *UserStore) RevokeSession(ctx context.Context, token string) error {
	if err := us.cache.Del(ctx, authhelper.SessionKey(token)).Err(); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (us *UserStore) Get(ctx context.Context, id uuid.UUID) (*User, error) {
	user := &User{}
	err := us.db.QueryRow(ctx, `
		SELECT id, email, name, created_at, updated_at
		FROM users
		WHERE id = $1
	`, id).Scan(&user.ID, &user.Email, &user.Name, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user: %w", err)
	}
	return user, nil
}

// Add creates a user and their password identity.
func (us *UserStore) Add(ctx context.Context, email, name, password string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)
	if email == "" || password == "" {
		return nil, fmt.Errorf("email and password are required")
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	tx, err := us.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin user creation: %w", err)
	}
	defer tx.Rollback(ctx)

	user := &User{ID: uuid.New(), Email: email, Name: name}
	err = tx.QueryRow(ctx, `
		INSERT INTO users (id, email, name)
		VALUES ($1, $2, $3)
		RETURNING created_at, updated_at
	`, user.ID, user.Email, user.Name).Scan(&user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if storehelper.IsUniqueViolation(err) {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("insert user: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO auth_identities (id, user_id, provider, provider_subject, password_hash)
		VALUES ($1, $2, 'password', $3, $4)
	`, uuid.New(), user.ID, user.Email, string(passwordHash))
	if err != nil {
		if storehelper.IsUniqueViolation(err) {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("insert password identity: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit user creation: %w", err)
	}
	return user, nil
}

// Update changes a user's email and display name.
func (us *UserStore) Update(ctx context.Context, id uuid.UUID, email, name string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}

	tx, err := us.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin user update: %w", err)
	}
	defer tx.Rollback(ctx)

	user := &User{}
	err = tx.QueryRow(ctx, `
		UPDATE users SET email = $2, name = $3, updated_at = now()
		WHERE id = $1
		RETURNING id, email, name, created_at, updated_at
	`, id, email, name).Scan(&user.ID, &user.Email, &user.Name, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		if storehelper.IsUniqueViolation(err) {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("update user: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE auth_identities
		SET provider_subject = $2
		WHERE user_id = $1 AND provider = 'password'
	`, id, email)
	if err != nil {
		if storehelper.IsUniqueViolation(err) {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("update password identity email: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit user update: %w", err)
	}
	return user, nil
}

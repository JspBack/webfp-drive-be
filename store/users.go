package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"

	storehelper "github.com/JspBack/webfp-drive/helper/store"
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrEmailTaken   = errors.New("email is already registered")
	ErrInvalidCreds = errors.New("invalid email or password")
)

type User struct {
	ID            uuid.UUID `json:"id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type UserStore struct {
	schemaTables
	db              *pgxpool.Pool
	cache           *redis.Client
	accessLifetime  time.Duration
	refreshLifetime time.Duration
}

func (us *UserStore) Authenticate(ctx context.Context, email, password string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	user := &User{}
	var passwordHash string
	err := us.db.QueryRow(ctx, `
		SELECT u.id, u.email, u.name, u.email_verified, u.created_at, u.updated_at, a.password_hash
		FROM `+us.table("users")+` u
		JOIN `+us.table("auth_identities")+` a ON a.user_id = u.id
		WHERE a.provider = 'password' AND a.provider_subject = $1
	`, email).Scan(
		&user.ID, &user.Email, &user.Name, &user.EmailVerified, &user.CreatedAt, &user.UpdatedAt, &passwordHash,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidCreds
		}
		return nil, fmt.Errorf("find login identity: %w", err)
	}
	if compareErr := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); compareErr != nil {
		return nil, ErrInvalidCreds
	}
	return user, nil
}

func (us *UserStore) OAuthLogin(ctx context.Context, provider, subject, email, name string) (*User, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return nil, fmt.Errorf("%s subject is required", provider)
	}
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)

	user := &User{}
	err := withTransaction(ctx, us.db, provider+" user login", func(tx pgx.Tx) error {
		lookupErr := tx.QueryRow(ctx, `
			SELECT u.id, COALESCE(u.email, ''), u.name, u.email_verified, u.created_at, u.updated_at
			FROM `+us.table("users")+` u
			JOIN `+us.table("auth_identities")+` a ON a.user_id = u.id
			WHERE a.provider = $2 AND a.provider_subject = $1
		`, subject, provider).Scan(&user.ID, &user.Email, &user.Name, &user.EmailVerified, &user.CreatedAt, &user.UpdatedAt)
		if lookupErr == nil {
			return nil
		}
		if !errors.Is(lookupErr, pgx.ErrNoRows) {
			return fmt.Errorf("find %s identity: %w", provider, lookupErr)
		}

		user.ID = uuid.New()
		user.Email = email
		user.Name = name
		user.EmailVerified = email != ""
		if createErr := tx.QueryRow(ctx, `
			INSERT INTO `+us.table("users")+` (id, email, name, email_verified)
			VALUES ($1, NULLIF($2, ''), $3, $4)
			RETURNING created_at, updated_at
		`, user.ID, user.Email, user.Name, user.EmailVerified).Scan(&user.CreatedAt, &user.UpdatedAt); createErr != nil {
			if storehelper.IsUniqueViolation(createErr) {
				return ErrEmailTaken
			}
			return fmt.Errorf("create %s user: %w", provider, createErr)
		}
		if _, identityErr := tx.Exec(ctx, `
			INSERT INTO `+us.table("auth_identities")+` (id, user_id, provider, provider_subject)
			VALUES ($1, $2, $3, $4)
		`, uuid.New(), user.ID, provider, subject); identityErr != nil {
			if storehelper.IsUniqueViolation(identityErr) {
				return ErrEmailTaken
			}
			return fmt.Errorf("save %s identity: %w", provider, identityErr)
		}
		return nil
	})
	if err == nil {
		return user, nil
	}
	if errors.Is(err, ErrEmailTaken) || storehelper.IsUniqueViolation(err) {
		existing, lookupErr := us.findOAuthUser(ctx, provider, subject)
		if lookupErr == nil {
			return existing, nil
		}
		if !errors.Is(lookupErr, ErrUserNotFound) {
			return nil, lookupErr
		}
		return nil, ErrEmailTaken
	}
	return nil, fmt.Errorf("login %s user: %w", provider, err)
}

func (us *UserStore) findOAuthUser(ctx context.Context, provider, subject string) (*User, error) {
	user := &User{}
	err := us.db.QueryRow(ctx, `
		SELECT u.id, COALESCE(u.email, ''), u.name, u.email_verified, u.created_at, u.updated_at
		FROM `+us.table("users")+` u
		JOIN `+us.table("auth_identities")+` a ON a.user_id = u.id
		WHERE a.provider = $2 AND a.provider_subject = $1
	`, strings.TrimSpace(subject), provider).Scan(
		&user.ID, &user.Email, &user.Name, &user.EmailVerified, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get %s user: %w", provider, err)
	}
	return user, nil
}
func (us *UserStore) Get(ctx context.Context, id uuid.UUID) (*User, error) {
	user := &User{}
	err := us.db.QueryRow(ctx, `
		SELECT id, COALESCE(email, ''), name, email_verified, created_at, updated_at
		FROM `+us.table("users")+`
		WHERE id = $1
	`, id).Scan(&user.ID, &user.Email, &user.Name, &user.EmailVerified, &user.CreatedAt, &user.UpdatedAt)
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
		return nil, errors.New("email and password are required")
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &User{ID: uuid.New(), Email: email, Name: name}
	err = withTransaction(ctx, us.db, "user creation", func(tx pgx.Tx) error {
		if queryErr := tx.QueryRow(ctx, `
			INSERT INTO `+us.table("users")+` (id, email, name, email_verified)
			VALUES ($1, $2, $3, false)
			RETURNING created_at, updated_at
		`, user.ID, user.Email, user.Name).Scan(&user.CreatedAt, &user.UpdatedAt); queryErr != nil {
			if storehelper.IsUniqueViolation(queryErr) {
				return ErrEmailTaken
			}
			return fmt.Errorf("insert user: %w", queryErr)
		}

		if _, execErr := tx.Exec(ctx, `
			INSERT INTO `+us.table("auth_identities")+` (id, user_id, provider, provider_subject, password_hash)
			VALUES ($1, $2, 'password', $3, $4)
		`, uuid.New(), user.ID, user.Email, string(passwordHash)); execErr != nil {
			if storehelper.IsUniqueViolation(execErr) {
				return ErrEmailTaken
			}
			return fmt.Errorf("insert password identity: %w", execErr)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("create user transaction: %w", err)
	}
	return user, nil
}

func withTransaction(
	ctx context.Context,
	db *pgxpool.Pool,
	operation string,
	run func(pgx.Tx) error,
) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin %s: %w", operation, err)
	}
	if runErr := run(tx); runErr != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			return errors.Join(runErr, fmt.Errorf("rollback %s: %w", operation, rollbackErr))
		}
		return runErr
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		wrappedCommitErr := fmt.Errorf("commit %s: %w", operation, commitErr)
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			return errors.Join(wrappedCommitErr, fmt.Errorf("rollback %s after commit failure: %w", operation, rollbackErr))
		}
		return wrappedCommitErr
	}
	return nil
}

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

	authhelper "github.com/JspBack/webfp-drive/helper/auth"
	storehelper "github.com/JspBack/webfp-drive/helper/store"
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrEmailTaken   = errors.New("email is already registered")
	ErrInvalidCreds = errors.New("invalid email or password")
)

var ErrInvalidSession = errors.New("invalid or expired session")

const (
	accessRecordParts  = 2
	refreshRecordParts = 3
	saveSessionScript  = `
redis.call("SET", KEYS[1], ARGV[1] .. "|" .. ARGV[2], "PX", ARGV[3])
redis.call("SET", KEYS[2], ARGV[1] .. "|" .. ARGV[2] .. "|" .. KEYS[1], "PX", ARGV[4])
redis.call("SET", KEYS[3], KEYS[2], "PX", ARGV[4])
return 1
`
	rotateSessionScript = `
local value = redis.call("GET", KEYS[1])
if not value or redis.call("GET", KEYS[2]) ~= KEYS[1] then
	return 0
end
local old_access_key = string.match(value, "^[^|]+|[^|]+|(.+)$")
if not old_access_key then
	return 0
end
redis.call("DEL", KEYS[1], old_access_key)
redis.call("SET", KEYS[3], ARGV[1] .. "|" .. ARGV[2], "PX", ARGV[3])
redis.call("SET", KEYS[4], ARGV[1] .. "|" .. ARGV[2] .. "|" .. KEYS[3], "PX", ARGV[4])
redis.call("SET", KEYS[2], KEYS[4], "PX", ARGV[4])
return 1
`
	revokeSessionScript = `
local refresh_key = redis.call("GET", KEYS[2])
if refresh_key then
	local refresh_value = redis.call("GET", refresh_key)
	local access_key = refresh_value and string.match(refresh_value, "^[^|]+|[^|]+|(.+)$")
	if access_key then
		redis.call("DEL", access_key)
	end
	redis.call("DEL", refresh_key)
end
redis.call("DEL", KEYS[1], KEYS[2])
return 1
`
)

type User struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
		SELECT u.id, u.email, u.name, u.created_at, u.updated_at, a.password_hash
		FROM `+us.table("users")+` u
		JOIN `+us.table("auth_identities")+` a ON a.user_id = u.id
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
	if compareErr := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); compareErr != nil {
		return nil, ErrInvalidCreds
	}
	return user, nil
}

func (us *UserStore) LoginGitHub(ctx context.Context, subject, email, name string) (*User, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return nil, errors.New("github subject is required")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)

	user := &User{}
	err := withTransaction(ctx, us.db, "GitHub user login", func(tx pgx.Tx) error {
		lookupErr := tx.QueryRow(ctx, `
			SELECT u.id, COALESCE(u.email, ''), u.name, u.created_at, u.updated_at
			FROM `+us.table("users")+` u
			JOIN `+us.table("auth_identities")+` a ON a.user_id = u.id
			WHERE a.provider = 'github' AND a.provider_subject = $1
		`, subject).Scan(&user.ID, &user.Email, &user.Name, &user.CreatedAt, &user.UpdatedAt)
		if lookupErr == nil {
			return nil
		}
		if !errors.Is(lookupErr, pgx.ErrNoRows) {
			return fmt.Errorf("find GitHub identity: %w", lookupErr)
		}

		user.ID = uuid.New()
		user.Email = email
		user.Name = name
		if createErr := tx.QueryRow(ctx, `
			INSERT INTO `+us.table("users")+` (id, email, name)
			VALUES ($1, NULLIF($2, ''), $3)
			RETURNING created_at, updated_at
		`, user.ID, user.Email, user.Name).Scan(&user.CreatedAt, &user.UpdatedAt); createErr != nil {
			if storehelper.IsUniqueViolation(createErr) {
				return ErrEmailTaken
			}
			return fmt.Errorf("create GitHub user: %w", createErr)
		}
		if _, identityErr := tx.Exec(ctx, `
			INSERT INTO `+us.table("auth_identities")+` (id, user_id, provider, provider_subject)
			VALUES ($1, $2, 'github', $3)
		`, uuid.New(), user.ID, subject); identityErr != nil {
			if storehelper.IsUniqueViolation(identityErr) {
				return ErrEmailTaken
			}
			return fmt.Errorf("save GitHub identity: %w", identityErr)
		}
		return nil
	})
	if err == nil {
		return user, nil
	}
	if errors.Is(err, ErrEmailTaken) || storehelper.IsUniqueViolation(err) {
		existing, lookupErr := us.findGitHubUser(ctx, subject)
		if lookupErr == nil {
			return existing, nil
		}
		if !errors.Is(lookupErr, ErrUserNotFound) {
			return nil, lookupErr
		}
		return nil, ErrEmailTaken
	}
	return nil, fmt.Errorf("login GitHub user: %w", err)
}

func (us *UserStore) findGitHubUser(ctx context.Context, subject string) (*User, error) {
	user := &User{}
	err := us.db.QueryRow(ctx, `
		SELECT u.id, COALESCE(u.email, ''), u.name, u.created_at, u.updated_at
		FROM `+us.table("users")+` u
		JOIN `+us.table("auth_identities")+` a ON a.user_id = u.id
		WHERE a.provider = 'github' AND a.provider_subject = $1
	`, strings.TrimSpace(subject)).Scan(
		&user.ID, &user.Email, &user.Name, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get GitHub user: %w", err)
	}
	return user, nil
}

func (us *UserStore) CreateSession(ctx context.Context, userID uuid.UUID) (*authhelper.TokenPair, error) {
	pair, err := authhelper.NewTokenPair()
	if err != nil {
		return nil, fmt.Errorf("create session tokens: %w", err)
	}
	if saveErr := us.cache.Eval(ctx, saveSessionScript,
		[]string{
			authhelper.SessionKey(pair.AccessToken),
			authhelper.RefreshKey(pair.RefreshToken),
			authhelper.SessionIndexKey(pair.SessionID),
		},
		userID.String(), pair.SessionID.String(),
		us.accessLifetime.Milliseconds(), us.refreshLifetime.Milliseconds(),
	).Err(); saveErr != nil {
		return nil, fmt.Errorf("save token pair: %w", saveErr)
	}
	return pair, nil
}

func (us *UserStore) ResolveSession(ctx context.Context, token string) (uuid.UUID, uuid.UUID, error) {
	value, err := us.cache.Get(ctx, authhelper.SessionKey(token)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return uuid.Nil, uuid.Nil, ErrInvalidSession
		}
		return uuid.Nil, uuid.Nil, fmt.Errorf("load session: %w", err)
	}
	parts := strings.SplitN(value, "|", accessRecordParts)
	if len(parts) != accessRecordParts {
		return uuid.Nil, uuid.Nil, ErrInvalidSession
	}
	userID, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, uuid.Nil, ErrInvalidSession
	}
	sessionID, err := uuid.Parse(parts[1])
	if err != nil {
		return uuid.Nil, uuid.Nil, ErrInvalidSession
	}
	return userID, sessionID, nil
}

func (us *UserStore) RefreshSession(ctx context.Context, refreshToken string) (*authhelper.TokenPair, error) {
	refreshKey := authhelper.RefreshKey(refreshToken)
	refreshValue, err := us.cache.Get(ctx, refreshKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrInvalidSession
		}
		return nil, fmt.Errorf("load refresh token: %w", err)
	}
	tokenParts := strings.SplitN(refreshValue, "|", 3)
	if len(tokenParts) != refreshRecordParts {
		return nil, errors.New("invalid refresh token record")
	}
	userIDValue := tokenParts[0]
	if _, parseErr := uuid.Parse(userIDValue); parseErr != nil {
		return nil, fmt.Errorf("parse refresh token user ID: %w", parseErr)
	}
	sessionID, err := uuid.Parse(tokenParts[1])
	if err != nil {
		return nil, fmt.Errorf("parse refresh token session ID: %w", err)
	}
	pair, err := authhelper.NewTokenPair()
	if err != nil {
		return nil, fmt.Errorf("create refreshed tokens: %w", err)
	}
	pair.SessionID = sessionID
	result, err := us.cache.Eval(ctx, rotateSessionScript,
		[]string{
			refreshKey,
			authhelper.SessionIndexKey(pair.SessionID),
			authhelper.SessionKey(pair.AccessToken),
			authhelper.RefreshKey(pair.RefreshToken),
		},
		userIDValue, pair.SessionID.String(),
		us.accessLifetime.Milliseconds(), us.refreshLifetime.Milliseconds(),
	).Int64()
	if err != nil {
		return nil, fmt.Errorf("rotate refresh token: %w", err)
	}
	if result == 0 {
		return nil, ErrInvalidSession
	}
	return pair, nil
}

func (us *UserStore) RevokeSession(ctx context.Context, token string, sessionID uuid.UUID) error {
	if err := us.cache.Eval(ctx, revokeSessionScript,
		[]string{
			authhelper.SessionKey(token),
			authhelper.SessionIndexKey(sessionID),
		},
	).Err(); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (us *UserStore) Get(ctx context.Context, id uuid.UUID) (*User, error) {
	user := &User{}
	err := us.db.QueryRow(ctx, `
		SELECT id, COALESCE(email, ''), name, created_at, updated_at
		FROM `+us.table("users")+`
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
		return nil, errors.New("email and password are required")
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &User{ID: uuid.New(), Email: email, Name: name}
	err = withTransaction(ctx, us.db, "user creation", func(tx pgx.Tx) error {
		if queryErr := tx.QueryRow(ctx, `
			INSERT INTO `+us.table("users")+` (id, email, name)
			VALUES ($1, $2, $3)
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

// Update changes a user's email and display name.
func (us *UserStore) Update(ctx context.Context, id uuid.UUID, email, name string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)
	if email == "" {
		return nil, errors.New("email is required")
	}

	user := &User{}
	err := withTransaction(ctx, us.db, "user update", func(tx pgx.Tx) error {
		if queryErr := tx.QueryRow(ctx, `
			UPDATE `+us.table("users")+` SET email = $2, name = $3, updated_at = now()
			WHERE id = $1
			RETURNING id, email, name, created_at, updated_at
		`, id, email, name).Scan(&user.ID, &user.Email, &user.Name, &user.CreatedAt, &user.UpdatedAt); queryErr != nil {
			if errors.Is(queryErr, pgx.ErrNoRows) {
				return ErrUserNotFound
			}
			if storehelper.IsUniqueViolation(queryErr) {
				return ErrEmailTaken
			}
			return fmt.Errorf("update user: %w", queryErr)
		}

		if _, execErr := tx.Exec(ctx, `
			UPDATE `+us.table("auth_identities")+`
			SET provider_subject = $2
			WHERE user_id = $1 AND provider = 'password'
		`, id, email); execErr != nil {
			if storehelper.IsUniqueViolation(execErr) {
				return ErrEmailTaken
			}
			return fmt.Errorf("update password identity email: %w", execErr)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("update user transaction: %w", err)
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

package store

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

const (
	verificationMaxAttempts = 5
	verificationAttemptsTTL = 15 * time.Minute
)

var (
	ErrVerificationInvalid   = errors.New("invalid verification code")
	ErrVerificationExpired   = errors.New("verification code has expired")
	ErrVerificationExhausted = errors.New("too many verification attempts")
)

func verificationCodeKey(email string) string {
	return "webfp:email_verify:code:" + email
}

func verificationAttemptsKey(email string) string {
	return "webfp:email_verify:attempts:" + email
}

func verificationCooldownKey(email string) string {
	return "webfp:email_verify:cooldown:" + email
}

func normalizeVerificationEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func hashVerificationCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func (us *UserStore) SetVerificationCode(ctx context.Context, email, code string, ttl time.Duration) error {
	email = normalizeVerificationEmail(email)
	if email == "" || code == "" || ttl <= 0 {
		return errors.New("email, code and ttl are required")
	}
	pipe := us.cache.TxPipeline()
	pipe.Set(ctx, verificationCodeKey(email), hashVerificationCode(code), ttl)
	pipe.Del(ctx, verificationAttemptsKey(email))
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("store verification code: %w", err)
	}
	return nil
}

func (us *UserStore) ConsumeVerificationCode(ctx context.Context, email, code string) error {
	email = normalizeVerificationEmail(email)
	if email == "" || code == "" {
		return ErrVerificationInvalid
	}
	stored, err := us.cache.Get(ctx, verificationCodeKey(email)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrVerificationExpired
		}
		return fmt.Errorf("load verification code: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(stored), []byte(hashVerificationCode(code))) != 1 {
		attempts, attemptErr := us.cache.Incr(ctx, verificationAttemptsKey(email)).Result()
		if attemptErr != nil {
			return fmt.Errorf("count verification attempt: %w", attemptErr)
		}
		if attempts == 1 {
			us.cache.Expire(ctx, verificationAttemptsKey(email), verificationAttemptsTTL)
		}
		if attempts >= verificationMaxAttempts {
			us.cache.Del(ctx, verificationCodeKey(email), verificationAttemptsKey(email))
			return ErrVerificationExhausted
		}
		return ErrVerificationInvalid
	}
	us.cache.Del(ctx, verificationCodeKey(email), verificationAttemptsKey(email))
	return nil
}

func (us *UserStore) TryVerificationCooldown(ctx context.Context, email string, window time.Duration) (bool, error) {
	email = normalizeVerificationEmail(email)
	if email == "" || window <= 0 {
		return false, errors.New("email and window are required")
	}
	allowed, err := us.cache.SetNX(ctx, verificationCooldownKey(email), "1", window).Result()
	if err != nil {
		return false, fmt.Errorf("check verification cooldown: %w", err)
	}
	return allowed, nil
}

func (us *UserStore) GetByEmail(ctx context.Context, email string) (*User, error) {
	email = normalizeVerificationEmail(email)
	if email == "" {
		return nil, ErrUserNotFound
	}
	user := &User{}
	err := us.db.QueryRow(ctx, `
		SELECT id, COALESCE(email, ''), name, email_verified, created_at, updated_at
		FROM `+us.table("users")+`
		WHERE lower(email) = $1
	`, email).Scan(&user.ID, &user.Email, &user.Name, &user.EmailVerified, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return user, nil
}

func (us *UserStore) MarkVerified(ctx context.Context, email string) (*User, error) {
	email = normalizeVerificationEmail(email)
	if email == "" {
		return nil, ErrUserNotFound
	}
	user := &User{}
	err := us.db.QueryRow(ctx, `
		UPDATE `+us.table("users")+`
		SET email_verified = true, updated_at = now()
		WHERE lower(email) = $1
		RETURNING id, COALESCE(email, ''), name, email_verified, created_at, updated_at
	`, email).Scan(&user.ID, &user.Email, &user.Name, &user.EmailVerified, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("mark email verified: %w", err)
	}
	return user, nil
}

func (us *UserStore) DeleteStaleUnverifiedUsers(ctx context.Context, olderThan time.Duration) (int64, error) {
	if olderThan <= 0 {
		return 0, errors.New("retention window is required")
	}
	tag, err := us.db.Exec(ctx, `
		DELETE FROM `+us.table("users")+`
		WHERE email_verified = false
			AND created_at < now() - make_interval(secs => $1)
	`, olderThan.Seconds())
	if err != nil {
		return 0, fmt.Errorf("delete stale unverified users: %w", err)
	}
	return tag.RowsAffected(), nil
}

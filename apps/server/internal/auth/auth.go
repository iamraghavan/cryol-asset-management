package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"
)

const SessionLifetime = 8 * time.Hour

var (
	ErrCredentials = errors.New("invalid credentials")
	ErrInitialized = errors.New("administrator bootstrap has already been completed")
	usernameFormat = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,63}$`)
)

type User struct {
	ID          string   `json:"id"`
	Username    string   `json:"username"`
	Permissions []string `json:"permissions"`
}

func (u User) Can(permission string) bool {
	for _, granted := range u.Permissions {
		if granted == permission {
			return true
		}
	}
	return false
}

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func derive(password, salt []byte) []byte {
	return argon2.IDKey(password, salt, 2, 19*1024, 1, 32)
}

func Bootstrap(ctx context.Context, pool *pgxpool.Pool, username string, password []byte) error {
	username = normalizeUsername(username)
	if !usernameFormat.MatchString(username) {
		return errors.New("username must contain 3–64 letters, digits, dots, underscores or hyphens and start with a letter or digit")
	}
	if !utf8.Valid(password) || utf8.RuneCount(password) < 15 || len(password) > 1024 {
		return errors.New("password must contain at least 15 characters and at most 1024 bytes")
	}
	salt := make([]byte, 16)
	rand.Read(salt)
	digest := derive(password, salt)
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT singleton FROM installation_info WHERE singleton FOR UPDATE"); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT FROM app_user)").Scan(&exists); err != nil {
			return err
		}
		if exists {
			return ErrInitialized
		}
		var id string
		if err := tx.QueryRow(ctx, "INSERT INTO app_user (username, password_salt, password_digest) VALUES ($1,$2,$3) RETURNING id::text", username, salt, digest).Scan(&id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO user_role VALUES ($1,'system-administrator')", id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "INSERT INTO audit_log (actor_id, event) VALUES ($1,'administrator.bootstrapped')", id)
		return err
	})
}

func Login(ctx context.Context, pool *pgxpool.Pool, username, password, previousToken string) (string, error) {
	username = normalizeUsername(username)
	if len(password) > 1024 || !usernameFormat.MatchString(username) {
		return "", ErrCredentials
	}
	var id string
	salt, digest := make([]byte, 16), make([]byte, 32)
	err := pool.QueryRow(ctx, "SELECT id::text, password_salt, password_digest FROM app_user WHERE username=$1 AND active", username).Scan(&id, &salt, &digest)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if subtle.ConstantTimeCompare(derive([]byte(password), salt), digest) != 1 || id == "" {
		return "", ErrCredentials
	}
	raw := make([]byte, 32)
	rand.Read(raw)
	hash := sha256.Sum256(raw)
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if old, err := sessionDigest(previousToken); err == nil {
			if _, err := tx.Exec(ctx, "DELETE FROM user_session WHERE token_digest=$1", old[:]); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, "DELETE FROM user_session WHERE expires_at <= now()"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO user_session VALUES ($1,$2,$3)", hash[:], id, time.Now().Add(SessionLifetime)); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "INSERT INTO audit_log (actor_id, event) VALUES ($1,'session.started')", id)
		return err
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func sessionDigest(token string) ([32]byte, error) {
	if len(token) != 43 {
		return [32]byte{}, ErrCredentials
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(raw) != 32 {
		return [32]byte{}, ErrCredentials
	}
	return sha256.Sum256(raw), nil
}

func Current(ctx context.Context, pool *pgxpool.Pool, token string) (User, error) {
	hash, err := sessionDigest(token)
	if err != nil {
		return User{}, err
	}
	var user User
	err = pool.QueryRow(ctx, `
SELECT u.id::text, u.username, ARRAY(
    SELECT DISTINCT rp.permission_code FROM user_role ur JOIN role_permission rp USING (role_code)
    WHERE ur.user_id = u.id ORDER BY rp.permission_code
)
FROM user_session s JOIN app_user u ON u.id=s.user_id
WHERE s.token_digest=$1 AND s.expires_at > now() AND u.active`, hash[:]).Scan(&user.ID, &user.Username, &user.Permissions)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrCredentials
	}
	return user, err
}

func Logout(ctx context.Context, pool *pgxpool.Pool, token string) error {
	hash, err := sessionDigest(token)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `WITH revoked AS (
DELETE FROM user_session WHERE token_digest=$1 RETURNING user_id
) INSERT INTO audit_log (actor_id,event) SELECT user_id,'session.ended' FROM revoked`, hash[:])
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

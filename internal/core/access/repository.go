package access

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jarviisha/codohue/pkg/codohuetypes"
	"golang.org/x/crypto/bcrypt"
)

// Store keeps authorization state in PostgreSQL so revocations survive replicas and restarts.
type Store struct{ db *pgxpool.Pool }

// NewStore constructs a shared PostgreSQL identity store.
func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

// Digest hashes a high-entropy token without persisting the bearer value.
func Digest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// RandomToken generates 32 cryptographically random bytes as hexadecimal.
func RandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", accessError(err)
	}
	return hex.EncodeToString(b), nil
}

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)

func passwordHash(password string) (string, error) {
	if len(password) < 12 || len(password) > 72 || password == "dev-secret-key" {
		return "", fmt.Errorf("%w: password must be 12–72 bytes and cannot be the known development credential", ErrInvalid)
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), accessError(err)
}

// Bootstrap creates exactly one initial owner; repeated starts never change credentials.
func (s *Store) Bootstrap(ctx context.Context, username, password string) error {
	if !namePattern.MatchString(username) {
		return ErrInvalid
	}
	hash, err := passwordHash(password)
	if err != nil {
		return accessError(err)
	}
	return accessError(pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(5757001)`); err != nil {
			return accessError(err)
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM admin_accounts`).Scan(&count); err != nil {
			return accessError(err)
		}
		if count > 0 {
			return nil
		}
		if _, err := tx.Exec(ctx, `INSERT INTO admin_accounts(username,password_hash,role) VALUES($1,$2,'owner')`, username, hash); err != nil {
			return accessError(err)
		}
		_, err := tx.Exec(ctx, `INSERT INTO admin_audit(actor,action,target) VALUES('bootstrap','account.create',$1)`, username)
		return accessError(err)
	}))
}

// SetAccount serializes owner changes to preserve at least one enabled owner.
// recovery is reserved for the local CLI with database authority.
func (s *Store) SetAccount(ctx context.Context, actor, username, password, role string, disabled, recovery bool) error {
	if !namePattern.MatchString(username) || (role != "owner" && role != "admin") {
		return ErrInvalid
	}
	hash := ""
	if password != "" {
		var err error
		hash, err = passwordHash(password)
		if err != nil {
			return accessError(err)
		}
	}
	return accessError(pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(5757001)`); err != nil {
			return accessError(err)
		}
		var oldRole string
		var oldDisabled bool
		err := tx.QueryRow(ctx, `SELECT role,disabled FROM admin_accounts WHERE username=$1 FOR UPDATE`, username).Scan(&oldRole, &oldDisabled)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return accessError(err)
		}
		if errors.Is(err, pgx.ErrNoRows) && hash == "" {
			return ErrInvalid
		}
		if oldRole == "owner" && !oldDisabled && (disabled || role != "owner") && !recovery {
			var owners int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM admin_accounts WHERE role='owner' AND NOT disabled`).Scan(&owners); err != nil {
				return accessError(err)
			}
			if owners <= 1 {
				return fmt.Errorf("%w: cannot disable or demote the last owner", ErrConflict)
			}
		}
		_, err = tx.Exec(ctx, `INSERT INTO admin_accounts(username,password_hash,role,disabled) VALUES($1,$2,$3,$4)
   ON CONFLICT(username) DO UPDATE SET password_hash=CASE WHEN $2='' THEN admin_accounts.password_hash ELSE $2 END,role=$3,disabled=$4,version=admin_accounts.version+1`, username, hash, role, disabled)
		if err != nil {
			return accessError(err)
		}
		if _, err = tx.Exec(ctx, `DELETE FROM admin_sessions WHERE username=$1`, username); err != nil {
			return accessError(err)
		}
		_, err = tx.Exec(ctx, `INSERT INTO admin_audit(actor,action,target) VALUES($1,'account.update',$2)`, actor, username)
		return accessError(err)
	}))
}

// Accounts lists enabled and disabled operator accounts.
func (s *Store) Accounts(ctx context.Context) ([]codohuetypes.OperatorAccount, error) {
	rows, err := s.db.Query(ctx, `SELECT username,role,disabled FROM admin_accounts ORDER BY username`)
	if err != nil {
		return nil, accessError(err)
	}
	defer rows.Close()
	out := []codohuetypes.OperatorAccount{}
	for rows.Next() {
		var a codohuetypes.OperatorAccount
		if err := rows.Scan(&a.Username, &a.Role, &a.Disabled); err != nil {
			return nil, accessError(err)
		}
		out = append(out, a)
	}
	return out, accessError(rows.Err())
}

// Login checks the shared budget before looking up or verifying any password.
func (s *Store) Login(ctx context.Context, ip, username, password string) (Actor, error) {
	ok, err := s.AllowAttempt(ctx, ip)
	if err != nil {
		return Actor{}, accessError(err)
	}
	if !ok {
		return Actor{}, ErrThrottled
	}
	var a Actor
	var hash string
	var disabled bool
	err = s.db.QueryRow(ctx, `SELECT username,role,password_hash,disabled,version FROM admin_accounts WHERE username=$1`, username).Scan(&a.Name, &a.Role, &hash, &disabled, &a.Version)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Actor{}, accessError(err)
	}
	if hash == "" {
		hash = dummyHash
	}
	valid := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
	if err != nil || !valid || disabled {
		return Actor{}, ErrCredentials
	}
	return a, nil
}

// A fixed valid hash makes unknown-account verification cost comparable to a real account.
const dummyHash = "$2a$10$7EqJtq98hPqEX7fNZaFWoO5RCPRgjfNL39..yhmEtV6ZQ2v9Z1e4S"

// ErrThrottled indicates that credential verification was not attempted.
var ErrThrottled = errors.New("too many login attempts")

// AllowAttempt atomically reserves a login attempt in the shared time window.
func (s *Store) AllowAttempt(ctx context.Context, ip string) (bool, error) {
	var attempts int
	err := s.db.QueryRow(ctx, `INSERT INTO admin_login_buckets(bucket,attempts,expires_at) VALUES($1,1,now()+interval '1 minute')
 ON CONFLICT(bucket) DO UPDATE SET attempts=CASE WHEN admin_login_buckets.expires_at<=now() THEN 1 ELSE LEAST(admin_login_buckets.attempts+1,6) END,
 expires_at=CASE WHEN admin_login_buckets.expires_at<=now() THEN now()+interval '1 minute' ELSE admin_login_buckets.expires_at END RETURNING attempts`, Digest(ip)).Scan(&attempts)
	return attempts <= 5, accessError(err)
}

// IssueSession binds a random session to the verified account version.
func (s *Store) IssueSession(ctx context.Context, a Actor) (string, time.Time, error) {
	token, err := RandomToken()
	if err != nil {
		return "", time.Time{}, accessError(err)
	}
	expires := time.Now().Add(8 * time.Hour)
	tag, err := s.db.Exec(ctx, `INSERT INTO admin_sessions(token_hash,username,account_version,expires_at)
 SELECT $1,username,version,$4 FROM admin_accounts WHERE username=$2 AND version=$3 AND NOT disabled`, Digest(token), a.Name, a.Version, expires)
	if err != nil {
		return "", time.Time{}, accessError(err)
	}
	if tag.RowsAffected() != 1 {
		return "", time.Time{}, ErrCredentials
	}
	return token, expires, nil
}

// Session validates expiry, revocation and live account state.
func (s *Store) Session(ctx context.Context, token string) (Actor, error) {
	var a Actor
	err := s.db.QueryRow(ctx, `SELECT a.username,a.role,a.version FROM admin_sessions s JOIN admin_accounts a ON a.username=s.username
 WHERE s.token_hash=$1 AND s.expires_at>now() AND NOT a.disabled AND a.version=s.account_version`, Digest(token)).Scan(&a.Name, &a.Role, &a.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrCredentials
	}
	return a, accessError(err)
}

// RevokeSession removes a session from the shared store.
func (s *Store) RevokeSession(ctx context.Context, token string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM admin_sessions WHERE token_hash=$1`, Digest(token))
	return accessError(err)
}

// ValidateTokenSpec checks token encoding and the supported permission vocabulary.
func ValidateTokenSpec(token string, permissions, namespaces []string) error {
	raw, err := hex.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return fmt.Errorf("%w: token must be 32 random bytes encoded as 64 hex characters", ErrInvalid)
	}
	if len(permissions) == 0 || len(namespaces) == 0 {
		return ErrInvalid
	}
	for _, p := range permissions {
		if !slices.Contains([]string{"admin:read", "admin:write", "namespace:provision", "data:read", "data:write"}, p) {
			return ErrInvalid
		}
	}
	for _, ns := range namespaces {
		if ns != "*" && !namePattern.MatchString(ns) {
			return ErrInvalid
		}
	}
	return nil
}

// ProvisionToken is immutable and retry-safe. Revoked tokens cannot be resurrected by startup.
func (s *Store) ProvisionToken(ctx context.Context, actor, name, token string, permissions, namespaces []string) error {
	if !namePattern.MatchString(name) {
		return ErrInvalid
	}
	if err := ValidateTokenSpec(token, permissions, namespaces); err != nil {
		return accessError(err)
	}
	return accessError(pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO admin_service_tokens(name,token_hash,permissions,namespaces) VALUES($1,$2,$3,$4) ON CONFLICT(name) DO NOTHING`, name, Digest(token), permissions, namespaces)
		if err != nil {
			var conflict *pgconn.PgError
			if errors.As(err, &conflict) && conflict.Code == "23505" {
				return ErrConflict
			}
			return accessError(err)
		}
		var hash string
		var ps, nss []string
		var revoked bool
		if err := tx.QueryRow(ctx, `SELECT token_hash,permissions,namespaces,revoked FROM admin_service_tokens WHERE name=$1`, name).Scan(&hash, &ps, &nss, &revoked); err != nil {
			return accessError(err)
		}
		slices.Sort(ps)
		slices.Sort(nss)
		permissions = slices.Clone(permissions)
		namespaces = slices.Clone(namespaces)
		slices.Sort(permissions)
		slices.Sort(namespaces)
		if hash != Digest(token) || revoked || !slices.Equal(ps, permissions) || !slices.Equal(nss, namespaces) {
			return ErrConflict
		}
		_, err = tx.Exec(ctx, `INSERT INTO admin_audit(actor,action,target) VALUES($1,'token.provision',$2)`, actor, name)
		return accessError(err)
	}))
}

// ServiceToken resolves an unrevoked machine credential.
func (s *Store) ServiceToken(ctx context.Context, token string) (Actor, error) {
	a := Actor{Role: "service"}
	err := s.db.QueryRow(ctx, `SELECT name,permissions,namespaces FROM admin_service_tokens WHERE token_hash=$1 AND NOT revoked`, Digest(token)).Scan(&a.Name, &a.Permissions, &a.Namespaces)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrCredentials
	}
	return a, accessError(err)
}

// RevokeToken permanently disables a named service token.
func (s *Store) RevokeToken(ctx context.Context, actor, name string) error {
	return accessError(pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE admin_service_tokens SET revoked=true WHERE name=$1`, name); err != nil {
			return accessError(err)
		}
		_, err := tx.Exec(ctx, `INSERT INTO admin_audit(actor,action,target) VALUES($1,'token.revoke',$2)`, actor, name)
		return accessError(err)
	}))
}

// Audit records actor and action metadata without credential material.
func (s *Store) Audit(ctx context.Context, actor, action, target string) error {
	_, err := s.db.Exec(ctx, `INSERT INTO admin_audit(actor,action,target) VALUES($1,$2,$3)`, actor, action, target)
	return accessError(err)
}

// Prune removes expired security state without removing durable audit records.
func (s *Store) Prune(ctx context.Context) error {
	if _, err := s.db.Exec(ctx, `DELETE FROM admin_sessions WHERE expires_at<=now()`); err != nil {
		return accessError(err)
	}
	_, err := s.db.Exec(ctx, `DELETE FROM admin_login_buckets WHERE expires_at<=now()-interval '1 hour'`)
	return accessError(err)
}

// RefundAttempt releases a reserved compatibility-bearer attempt after verification succeeds.
// It never allows a credential to be checked after the shared budget is exhausted.
func (s *Store) RefundAttempt(ctx context.Context, ip string) error {
	_, err := s.db.Exec(ctx, `UPDATE admin_login_buckets SET attempts=GREATEST(attempts-1,0) WHERE bucket=$1`, Digest(ip))
	return accessError(err)
}

func accessError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("access store: %w", err)
}

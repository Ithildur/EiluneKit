// Package passkeyexample demonstrates application-owned credential persistence.
// Package passkeyexample 演示应用自行管理的凭据持久化。
package passkeyexample

import (
	"context"
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"time"

	"github.com/Ithildur/EiluneKit/auth/passkey"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store uses schema.sql. The application owns DB and fixes RPID at construction.
// Store 使用 schema.sql。应用负责 DB 生命周期，并在构造时固定 RPID。
type Store struct {
	DB   *pgxpool.Pool
	RPID string
}

var _ passkey.Store = (*Store)(nil)

// Insert preserves credential ownership by rejecting duplicate IDs.
// Insert 拒绝重复 ID，保证凭据归属不变。
func (s *Store) Insert(ctx context.Context, c passkey.Credential) error {
	data, err := json.Marshal(c.Data)
	if err != nil {
		return err
	}
	result, err := s.DB.Exec(ctx, `INSERT INTO passkeys (rp_id,id,user_id,name,created_at,credential)
VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (rp_id,id) DO NOTHING`, s.RPID, base64.RawURLEncoding.EncodeToString(c.Data.ID), c.UserID, c.Name, c.CreatedAt, data)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return passkey.ErrConflict
	}
	return nil
}

// Get loads a credential by its base64url ID.
// Get 按 base64url ID 加载凭据。
func (s *Store) Get(ctx context.Context, id string) (passkey.Credential, bool, error) {
	c, err := scan(s.DB.QueryRow(ctx, `SELECT user_id,name,created_at,last_used_at,credential FROM passkeys WHERE rp_id=$1 AND id=$2`, s.RPID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return passkey.Credential{}, false, nil
	}
	return c, err == nil, err
}

// List loads credentials for one user.
// List 加载单个用户的凭据。
func (s *Store) List(ctx context.Context, id string) ([]passkey.Credential, error) {
	rows, err := s.DB.Query(ctx, `SELECT user_id,name,created_at,last_used_at,credential FROM passkeys WHERE rp_id=$1 AND user_id=$2 ORDER BY created_at,id`, s.RPID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []passkey.Credential
	for rows.Next() {
		c, err := scan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func scan(row pgx.Row) (passkey.Credential, error) {
	var c passkey.Credential
	var lastUsed *time.Time
	var data []byte
	if err := row.Scan(&c.UserID, &c.Name, &c.CreatedAt, &lastUsed, &data); err != nil {
		return c, err
	}
	if lastUsed != nil {
		c.LastUsedAt = *lastUsed
	}
	err := json.Unmarshal(data, &c.Data)
	return c, err
}

// UpdateUsage locks the existing row and preserves its key, owner, and name.
// UpdateUsage 锁定现有记录，并保留其密钥、所有者和名称。
func (s *Store) UpdateUsage(ctx context.Context, id, userID string, a webauthn.Authenticator, flags webauthn.CredentialFlags, at time.Time) (bool, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	var data []byte
	err = tx.QueryRow(ctx, `SELECT credential FROM passkeys WHERE rp_id=$1 AND id=$2 AND user_id=$3 FOR UPDATE`, s.RPID, id, userID).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var c webauthn.Credential
	if err = json.Unmarshal(data, &c); err != nil {
		return false, err
	}
	c.Authenticator.SignCount = max(c.Authenticator.SignCount, a.SignCount)
	c.Authenticator.CloneWarning = c.Authenticator.CloneWarning || a.CloneWarning
	c.Flags.BackupState = flags.BackupState
	c.Flags.UserPresent = flags.UserPresent
	c.Flags.UserVerified = c.Flags.UserVerified || flags.UserVerified
	data, err = json.Marshal(c)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE passkeys SET credential=$4,last_used_at=GREATEST(last_used_at,$5) WHERE rp_id=$1 AND id=$2 AND user_id=$3`, s.RPID, id, userID, data, at)
	if err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// Rename updates only the requested user's credential.
// Rename 仅修改指定用户的凭据。
func (s *Store) Rename(ctx context.Context, id, userID, name string) (bool, error) {
	result, err := s.DB.Exec(ctx, `UPDATE passkeys SET name=$4 WHERE rp_id=$1 AND id=$2 AND user_id=$3`, s.RPID, id, userID, name)
	return result.RowsAffected() != 0, err
}

// Delete removes only the requested user's credential.
// Delete 仅删除指定用户的凭据。
func (s *Store) Delete(ctx context.Context, id, userID string) (bool, error) {
	result, err := s.DB.Exec(ctx, `DELETE FROM passkeys WHERE rp_id=$1 AND id=$2 AND user_id=$3`, s.RPID, id, userID)
	return result.RowsAffected() != 0, err
}

package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"backend/internal/repo"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

type Store struct {
	db *sql.DB
	q  executor
}

type executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func NewStore(db *sql.DB) *Store { return &Store{db: db, q: db} }

func (s *Store) Users() repo.UserRepo           { return &userRepo{q: s.q} }
func (s *Store) OAuth() repo.OAuthRepo          { return &oauthRepo{q: s.q} }
func (s *Store) Todos() repo.TodoRepo           { return &todoRepo{q: s.q} }
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }
func (s *Store) Close()                         { _ = s.db.Close() }

func (s *Store) WithUserOAuthTx(ctx context.Context, fn func(repo.UserRepo, repo.OAuthRepo) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	txStore := &Store{db: s.db, q: tx}
	if err := fn(txStore.Users(), txStore.OAuth()); err != nil {
		return err
	}
	return tx.Commit()
}

func wrapErr(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return repo.ErrNotFound
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return repo.ErrAlreadyExists
	}
	return err
}

// userColumns 返回带表前缀的 users 查询列表。单表查询传空串；
// 与 user_oauth JOIN 时必须传 "u."——两表同有 id/created_at，
// 裸列名会报 Error 1052 (ambiguous)，与 PG 侧 SELECT u.* 行为对齐。
func userColumns(prefix string) string {
	return prefix + strings.Join([]string{
		"id", "phone", "email", "password_hash", "nickname", "avatar",
		"role", "status", "created_at", "updated_at",
	}, ","+prefix)
}

type userRepo struct{ q executor }

func (r *userRepo) GetByID(ctx context.Context, id uuid.UUID) (*repo.User, error) {
	return scanUser(r.q.QueryRowContext(ctx, `SELECT `+userColumns("")+` FROM users WHERE id=? LIMIT 1`, id.String()))
}
func (r *userRepo) GetByPhone(ctx context.Context, phone string) (*repo.User, error) {
	return scanUser(r.q.QueryRowContext(ctx, `SELECT `+userColumns("")+` FROM users WHERE phone=? LIMIT 1`, phone))
}
func (r *userRepo) GetByEmail(ctx context.Context, email string) (*repo.User, error) {
	return scanUser(r.q.QueryRowContext(ctx, `SELECT `+userColumns("")+` FROM users WHERE email=? LIMIT 1`, email))
}
func (r *userRepo) Create(ctx context.Context, in *repo.CreateUserParams) (*repo.User, error) {
	if in == nil {
		return nil, errors.New("create user params is nil")
	}
	id := uuid.New()
	_, err := r.q.ExecContext(ctx, `INSERT INTO users (id, phone, email, password_hash, nickname, avatar, role) VALUES (?, ?, ?, ?, ?, ?, ?)`, id.String(), in.Phone, in.Email, in.PasswordHash, in.Nickname, in.Avatar, in.Role)
	if err != nil {
		return nil, wrapErr(err)
	}
	return r.GetByID(ctx, id)
}
func (r *userRepo) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) (*repo.User, error) {
	return r.update(ctx, id, `UPDATE users SET password_hash=?, updated_at=CURRENT_TIMESTAMP(6) WHERE id=?`, passwordHash, id.String())
}
func (r *userRepo) UpdateProfile(ctx context.Context, id uuid.UUID, nickname, avatar string) (*repo.User, error) {
	return r.update(ctx, id, `UPDATE users SET nickname=?, avatar=?, updated_at=CURRENT_TIMESTAMP(6) WHERE id=?`, nickname, avatar, id.String())
}
func (r *userRepo) UpdateRole(ctx context.Context, id uuid.UUID, role string) (*repo.User, error) {
	return r.update(ctx, id, `UPDATE users SET role=?, updated_at=CURRENT_TIMESTAMP(6) WHERE id=?`, role, id.String())
}
func (r *userRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) (*repo.User, error) {
	return r.update(ctx, id, `UPDATE users SET status=?, updated_at=CURRENT_TIMESTAMP(6) WHERE id=?`, status, id.String())
}
func (r *userRepo) update(ctx context.Context, id uuid.UUID, query string, args ...any) (*repo.User, error) {
	if _, err := r.q.ExecContext(ctx, query, args...); err != nil {
		return nil, wrapErr(err)
	}
	return r.GetByID(ctx, id)
}
func (r *userRepo) List(ctx context.Context, p *repo.ListUsersParams) ([]*repo.User, error) {
	if p == nil {
		return nil, errors.New("list users params is nil")
	}
	pattern := "%" + p.Keyword + "%"
	rows, err := r.q.QueryContext(ctx, `SELECT `+userColumns("")+` FROM users WHERE (?='' OR phone LIKE ? OR email LIKE ? OR nickname LIKE ?) AND (?='' OR status=?) ORDER BY created_at DESC LIMIT ? OFFSET ?`, p.Keyword, pattern, pattern, pattern, p.Status, p.Status, p.Limit, p.Offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	users := make([]*repo.User, 0)
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
func (r *userRepo) Count(ctx context.Context, keyword, status string) (int64, error) {
	var n int64
	err := r.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE (?='' OR phone LIKE ? OR email LIKE ? OR nickname LIKE ?) AND (?='' OR status=?)`, keyword, "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%", status, status).Scan(&n)
	return n, err
}

type scanner interface{ Scan(...any) error }

func scanUser(row scanner) (*repo.User, error) {
	u := new(repo.User)
	var id string
	err := row.Scan(&id, &u.Phone, &u.Email, &u.PasswordHash, &u.Nickname, &u.Avatar, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, wrapErr(err)
	}
	u.ID, err = uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("parse user id: %w", err)
	}
	return u, nil
}

type oauthRepo struct{ q executor }

func (r *oauthRepo) GetOAuth(ctx context.Context, provider, openID string) (*repo.UserOAuth, error) {
	o := new(repo.UserOAuth)
	var id, userID string
	err := r.q.QueryRowContext(ctx, `SELECT id, user_id, provider, open_id, created_at FROM user_oauth WHERE provider=? AND open_id=? LIMIT 1`, provider, openID).Scan(&id, &userID, &o.Provider, &o.OpenID, &o.CreatedAt)
	if err != nil {
		return nil, wrapErr(err)
	}
	o.ID, err = uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	o.UserID, err = uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	return o, nil
}
func (r *oauthRepo) CreateOAuth(ctx context.Context, userID uuid.UUID, provider, openID string) (*repo.UserOAuth, error) {
	id := uuid.New()
	_, err := r.q.ExecContext(ctx, `INSERT INTO user_oauth (id, user_id, provider, open_id) VALUES (?, ?, ?, ?)`, id.String(), userID.String(), provider, openID)
	if err != nil {
		return nil, wrapErr(err)
	}
	return r.GetOAuth(ctx, provider, openID)
}
func (r *oauthRepo) GetUserByOAuth(ctx context.Context, provider, openID string) (*repo.User, error) {
	return scanUser(r.q.QueryRowContext(ctx, `SELECT `+userColumns("u.")+` FROM users u JOIN user_oauth o ON o.user_id=u.id WHERE o.provider=? AND o.open_id=? LIMIT 1`, provider, openID))
}

type todoRepo struct{ q executor }

func scanTodo(row scanner) (*repo.Todo, error) {
	t := new(repo.Todo)
	var id, userID string
	err := row.Scan(&id, &userID, &t.Title, &t.Done, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, wrapErr(err)
	}
	t.ID, err = uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	t.UserID, err = uuid.Parse(userID)
	return t, err
}
func (r *todoRepo) CountByUserID(ctx context.Context, id uuid.UUID) (int64, error) {
	var n int64
	err := r.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM todos WHERE user_id=?`, id.String()).Scan(&n)
	return n, err
}
func (r *todoRepo) ListByUserIDPaginated(ctx context.Context, id uuid.UUID, limit, offset int32) ([]repo.Todo, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT id, user_id, title, done, created_at, updated_at FROM todos WHERE user_id=? ORDER BY created_at DESC LIMIT ? OFFSET ?`, id.String(), limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]repo.Todo, 0)
	for rows.Next() {
		t, err := scanTodo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}
func (r *todoRepo) GetByIDAndUserID(ctx context.Context, id, userID uuid.UUID) (*repo.Todo, error) {
	return scanTodo(r.q.QueryRowContext(ctx, `SELECT id, user_id, title, done, created_at, updated_at FROM todos WHERE id=? AND user_id=? LIMIT 1`, id.String(), userID.String()))
}
func (r *todoRepo) Create(ctx context.Context, userID uuid.UUID, title string) (*repo.Todo, error) {
	id := uuid.New()
	_, err := r.q.ExecContext(ctx, `INSERT INTO todos (id, user_id, title) VALUES (?, ?, ?)`, id.String(), userID.String(), title)
	if err != nil {
		return nil, err
	}
	return r.GetByIDAndUserID(ctx, id, userID)
}
func (r *todoRepo) Update(ctx context.Context, id, userID uuid.UUID, title string, done bool) (*repo.Todo, error) {
	_, err := r.q.ExecContext(ctx, `UPDATE todos SET title=?, done=?, updated_at=CURRENT_TIMESTAMP(6) WHERE id=? AND user_id=?`, title, done, id.String(), userID.String())
	if err != nil {
		return nil, err
	}
	return r.GetByIDAndUserID(ctx, id, userID)
}
func (r *todoRepo) Delete(ctx context.Context, id, userID uuid.UUID) error {
	res, err := r.q.ExecContext(ctx, `DELETE FROM todos WHERE id=? AND user_id=?`, id.String(), userID.String())
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return repo.ErrNotFound
	}
	return nil
}

var _ repo.Storage = (*Store)(nil)

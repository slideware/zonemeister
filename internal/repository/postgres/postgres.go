package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"zonemeister/internal/models"
	"zonemeister/internal/repository"

	_ "github.com/lib/pq"
)

// OpenDB opens a PostgreSQL database with the given connection string.
func OpenDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return db, nil
}

// --- UserRepo ---

// UserRepo implements repository.UserRepository for PostgreSQL.
type UserRepo struct {
	db *sql.DB
}

func NewUserRepo(db *sql.DB) repository.UserRepository {
	return &UserRepo{db: db}
}

func (r *UserRepo) GetByID(ctx context.Context, id int64) (*models.User, error) {
	u := &models.User{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, role, customer_id, totp_secret, totp_enabled, created_at, updated_at FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CustomerID, &u.TOTPSecret, &u.TOTPEnabled, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return u, nil
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	u := &models.User{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, role, customer_id, totp_secret, totp_enabled, created_at, updated_at FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CustomerID, &u.TOTPSecret, &u.TOTPEnabled, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return u, nil
}

func (r *UserRepo) Create(ctx context.Context, user *models.User) error {
	now := time.Now().UTC()
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO users (email, password_hash, role, customer_id, totp_secret, totp_enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		user.Email, user.PasswordHash, user.Role, user.CustomerID, user.TOTPSecret, user.TOTPEnabled, now, now,
	).Scan(&user.ID)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	user.CreatedAt = now
	user.UpdatedAt = now
	return nil
}

func (r *UserRepo) Update(ctx context.Context, user *models.User) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET email = $1, password_hash = $2, role = $3, customer_id = $4, totp_secret = $5, totp_enabled = $6, updated_at = $7 WHERE id = $8`,
		user.Email, user.PasswordHash, user.Role, user.CustomerID, user.TOTPSecret, user.TOTPEnabled, now, user.ID,
	)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	user.UpdatedAt = now
	return nil
}

func (r *UserRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

func (r *UserRepo) ListByCustomerID(ctx context.Context, customerID int64) ([]*models.User, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, email, password_hash, role, customer_id, totp_secret, totp_enabled, created_at, updated_at FROM users WHERE customer_id = $1`, customerID,
	)
	if err != nil {
		return nil, fmt.Errorf("list users by customer: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		u := &models.User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CustomerID, &u.TOTPSecret, &u.TOTPEnabled, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// --- CustomerRepo ---

// CustomerRepo implements repository.CustomerRepository for PostgreSQL.
type CustomerRepo struct {
	db *sql.DB
}

func NewCustomerRepo(db *sql.DB) repository.CustomerRepository {
	return &CustomerRepo{db: db}
}

func (r *CustomerRepo) GetByID(ctx context.Context, id int64) (*models.Customer, error) {
	c := &models.Customer{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, email, notes, created_at, updated_at FROM customers WHERE id = $1`, id,
	).Scan(&c.ID, &c.Name, &c.Email, &c.Notes, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get customer by id: %w", err)
	}
	return c, nil
}

func (r *CustomerRepo) List(ctx context.Context) ([]*models.Customer, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, email, notes, created_at, updated_at FROM customers ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("list customers: %w", err)
	}
	defer rows.Close()

	var customers []*models.Customer
	for rows.Next() {
		c := &models.Customer{}
		if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.Notes, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan customer: %w", err)
		}
		customers = append(customers, c)
	}
	return customers, rows.Err()
}

func (r *CustomerRepo) Create(ctx context.Context, customer *models.Customer) error {
	now := time.Now().UTC()
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO customers (name, email, notes, created_at, updated_at) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		customer.Name, customer.Email, customer.Notes, now, now,
	).Scan(&customer.ID)
	if err != nil {
		return fmt.Errorf("create customer: %w", err)
	}
	customer.CreatedAt = now
	customer.UpdatedAt = now
	return nil
}

func (r *CustomerRepo) Update(ctx context.Context, customer *models.Customer) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`UPDATE customers SET name = $1, email = $2, notes = $3, updated_at = $4 WHERE id = $5`,
		customer.Name, customer.Email, customer.Notes, now, customer.ID,
	)
	if err != nil {
		return fmt.Errorf("update customer: %w", err)
	}
	customer.UpdatedAt = now
	return nil
}

func (r *CustomerRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM customers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete customer: %w", err)
	}
	return nil
}

// --- ZoneAssignmentRepo ---

// ZoneAssignmentRepo implements repository.ZoneAssignmentRepository for PostgreSQL.
type ZoneAssignmentRepo struct {
	db *sql.DB
}

func NewZoneAssignmentRepo(db *sql.DB) repository.ZoneAssignmentRepository {
	return &ZoneAssignmentRepo{db: db}
}

func (r *ZoneAssignmentRepo) ListByCustomerID(ctx context.Context, customerID int64) ([]*models.ZoneAssignment, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, customer_id, zone_id, zone_name, assigned_at FROM zone_assignments WHERE customer_id = $1`, customerID,
	)
	if err != nil {
		return nil, fmt.Errorf("list zone assignments by customer: %w", err)
	}
	defer rows.Close()

	var assignments []*models.ZoneAssignment
	for rows.Next() {
		a := &models.ZoneAssignment{}
		if err := rows.Scan(&a.ID, &a.CustomerID, &a.ZoneID, &a.ZoneName, &a.AssignedAt); err != nil {
			return nil, fmt.Errorf("scan zone assignment: %w", err)
		}
		assignments = append(assignments, a)
	}
	return assignments, rows.Err()
}

func (r *ZoneAssignmentRepo) ListAll(ctx context.Context) ([]*models.ZoneAssignment, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, customer_id, zone_id, zone_name, assigned_at FROM zone_assignments`,
	)
	if err != nil {
		return nil, fmt.Errorf("list all zone assignments: %w", err)
	}
	defer rows.Close()

	var assignments []*models.ZoneAssignment
	for rows.Next() {
		a := &models.ZoneAssignment{}
		if err := rows.Scan(&a.ID, &a.CustomerID, &a.ZoneID, &a.ZoneName, &a.AssignedAt); err != nil {
			return nil, fmt.Errorf("scan zone assignment: %w", err)
		}
		assignments = append(assignments, a)
	}
	return assignments, rows.Err()
}

func (r *ZoneAssignmentRepo) Assign(ctx context.Context, assignment *models.ZoneAssignment) error {
	now := time.Now().UTC()
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO zone_assignments (customer_id, zone_id, zone_name, assigned_at) VALUES ($1, $2, $3, $4) RETURNING id`,
		assignment.CustomerID, assignment.ZoneID, assignment.ZoneName, now,
	).Scan(&assignment.ID)
	if err != nil {
		return fmt.Errorf("assign zone: %w", err)
	}
	assignment.AssignedAt = now
	return nil
}

func (r *ZoneAssignmentRepo) Unassign(ctx context.Context, customerID int64, zoneID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM zone_assignments WHERE customer_id = $1 AND zone_id = $2`, customerID, zoneID,
	)
	if err != nil {
		return fmt.Errorf("unassign zone: %w", err)
	}
	return nil
}

func (r *ZoneAssignmentRepo) GetCustomerForZone(ctx context.Context, zoneID string) (*models.Customer, error) {
	c := &models.Customer{}
	err := r.db.QueryRowContext(ctx,
		`SELECT c.id, c.name, c.email, c.notes, c.created_at, c.updated_at
		 FROM customers c
		 JOIN zone_assignments za ON za.customer_id = c.id
		 WHERE za.zone_id = $1`, zoneID,
	).Scan(&c.ID, &c.Name, &c.Email, &c.Notes, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get customer for zone: %w", err)
	}
	return c, nil
}

func (r *ZoneAssignmentRepo) IsZoneAssigned(ctx context.Context, zoneID string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM zone_assignments WHERE zone_id = $1`, zoneID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check zone assigned: %w", err)
	}
	return count > 0, nil
}

// --- CustomerTSIGKeyRepo ---

// CustomerTSIGKeyRepo implements repository.CustomerTSIGKeyRepository for PostgreSQL.
type CustomerTSIGKeyRepo struct {
	db *sql.DB
}

func NewCustomerTSIGKeyRepo(db *sql.DB) repository.CustomerTSIGKeyRepository {
	return &CustomerTSIGKeyRepo{db: db}
}

func (r *CustomerTSIGKeyRepo) ListByCustomerID(ctx context.Context, customerID int64) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT tsig_key_name FROM customer_tsig_keys WHERE customer_id = $1 ORDER BY tsig_key_name`, customerID,
	)
	if err != nil {
		return nil, fmt.Errorf("list tsig keys by customer: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan tsig key name: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func (r *CustomerTSIGKeyRepo) SetForCustomer(ctx context.Context, customerID int64, keyNames []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM customer_tsig_keys WHERE customer_id = $1`, customerID); err != nil {
		return fmt.Errorf("delete existing tsig keys: %w", err)
	}

	for _, name := range keyNames {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO customer_tsig_keys (customer_id, tsig_key_name) VALUES ($1, $2)`,
			customerID, name,
		); err != nil {
			return fmt.Errorf("insert tsig key %q: %w", name, err)
		}
	}

	return tx.Commit()
}

// --- SessionRepo ---

// SessionRepo implements repository.SessionRepository for PostgreSQL.
type SessionRepo struct {
	db *sql.DB
}

func NewSessionRepo(db *sql.DB) repository.SessionRepository {
	return &SessionRepo{db: db}
}

func (r *SessionRepo) Create(ctx context.Context, session *models.Session) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, expires_at, created_at) VALUES ($1, $2, $3, $4)`,
		session.ID, session.UserID, session.ExpiresAt, session.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (r *SessionRepo) GetByID(ctx context.Context, id string) (*models.Session, error) {
	s := &models.Session{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, expires_at, created_at FROM sessions WHERE id = $1`, id,
	).Scan(&s.ID, &s.UserID, &s.ExpiresAt, &s.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session by id: %w", err)
	}
	return s, nil
}

func (r *SessionRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (r *SessionRepo) DeleteByUserID(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete sessions by user: %w", err)
	}
	return nil
}

func (r *SessionRepo) DeleteExpired(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < $1`, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}

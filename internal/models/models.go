package models

import "time"

// Role represents a user role.
type Role string

const (
	RoleSuperAdmin Role = "superadmin"
	RoleCustomer   Role = "customer"
)

// User represents an application user.
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	Role         Role
	CustomerID   *int64
	TOTPSecret   string
	TOTPEnabled  bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// IsSuperAdmin returns true if the user has the superadmin role.
func (u *User) IsSuperAdmin() bool {
	return u.Role == RoleSuperAdmin
}

// Customer represents a customer organization.
type Customer struct {
	ID        int64
	Name      string
	Email     string
	Notes     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ZoneAssignment links a customer to a DNS zone.
type ZoneAssignment struct {
	ID         int64
	CustomerID int64
	ZoneID     string
	ZoneName   string
	AssignedAt time.Time
}

// Session represents an authenticated user session.
type Session struct {
	ID        string
	UserID    int64
	ExpiresAt time.Time
	CreatedAt time.Time
}

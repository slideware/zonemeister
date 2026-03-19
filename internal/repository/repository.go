package repository

import (
	"context"

	"zonemeister/internal/models"
)

// UserRepository defines operations for user persistence.
type UserRepository interface {
	GetByID(ctx context.Context, id int64) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	Create(ctx context.Context, user *models.User) error
	Update(ctx context.Context, user *models.User) error
	Delete(ctx context.Context, id int64) error
	ListByCustomerID(ctx context.Context, customerID int64) ([]*models.User, error)
}

// CustomerRepository defines operations for customer persistence.
type CustomerRepository interface {
	GetByID(ctx context.Context, id int64) (*models.Customer, error)
	List(ctx context.Context) ([]*models.Customer, error)
	Create(ctx context.Context, customer *models.Customer) error
	Update(ctx context.Context, customer *models.Customer) error
	Delete(ctx context.Context, id int64) error
}

// ZoneAssignmentRepository defines operations for zone assignment persistence.
type ZoneAssignmentRepository interface {
	ListByCustomerID(ctx context.Context, customerID int64) ([]*models.ZoneAssignment, error)
	ListAll(ctx context.Context) ([]*models.ZoneAssignment, error)
	Assign(ctx context.Context, assignment *models.ZoneAssignment) error
	Unassign(ctx context.Context, customerID int64, zoneID string) error
	GetCustomerForZone(ctx context.Context, zoneID string) (*models.Customer, error)
	IsZoneAssigned(ctx context.Context, zoneID string) (bool, error)
}

// CustomerTSIGKeyRepository defines operations for customer TSIG key persistence.
type CustomerTSIGKeyRepository interface {
	ListByCustomerID(ctx context.Context, customerID int64) ([]string, error)
	SetForCustomer(ctx context.Context, customerID int64, keyNames []string) error
}

// SessionRepository defines operations for session persistence.
type SessionRepository interface {
	Create(ctx context.Context, session *models.Session) error
	GetByID(ctx context.Context, id string) (*models.Session, error)
	Delete(ctx context.Context, id string) error
	DeleteByUserID(ctx context.Context, userID int64) error
	DeleteExpired(ctx context.Context) error
}

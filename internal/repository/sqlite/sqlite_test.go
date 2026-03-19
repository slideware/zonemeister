package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"zonemeister/internal/models"
	"zonemeister/internal/repository/sqlite"
	"zonemeister/migrations"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlite.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := sqlite.RunMigrations(db, migrations.SQLiteFS()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// --- Customer tests ---

func TestCustomerCRUD(t *testing.T) {
	db := setupTestDB(t)
	repo := sqlite.NewCustomerRepo(db)
	ctx := context.Background()

	// Create
	c := &models.Customer{Name: "Acme Corp", Email: "acme@example.com", Notes: "Test customer"}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("create customer: %v", err)
	}
	if c.ID == 0 {
		t.Fatal("expected non-zero ID after create")
	}

	// GetByID
	got, err := repo.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("get customer: %v", err)
	}
	if got.Name != "Acme Corp" {
		t.Errorf("name = %q, want %q", got.Name, "Acme Corp")
	}
	if got.Email != "acme@example.com" {
		t.Errorf("email = %q, want %q", got.Email, "acme@example.com")
	}

	// Update
	c.Name = "Acme Inc"
	if err := repo.Update(ctx, c); err != nil {
		t.Fatalf("update customer: %v", err)
	}
	got, _ = repo.GetByID(ctx, c.ID)
	if got.Name != "Acme Inc" {
		t.Errorf("updated name = %q, want %q", got.Name, "Acme Inc")
	}

	// List
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list customers: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("list len = %d, want 1", len(list))
	}

	// Delete
	if err := repo.Delete(ctx, c.ID); err != nil {
		t.Fatalf("delete customer: %v", err)
	}
	got, _ = repo.GetByID(ctx, c.ID)
	if got != nil {
		t.Error("expected nil after delete")
	}
}

// --- User tests ---

func TestUserCRUD(t *testing.T) {
	db := setupTestDB(t)
	userRepo := sqlite.NewUserRepo(db)
	custRepo := sqlite.NewCustomerRepo(db)
	ctx := context.Background()

	// Create a customer first.
	cust := &models.Customer{Name: "Test Co"}
	custRepo.Create(ctx, cust)

	// Create user
	u := &models.User{
		Email:        "user@example.com",
		PasswordHash: "fakehash",
		Role:         models.RoleCustomer,
		CustomerID:   &cust.ID,
	}
	if err := userRepo.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if u.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	// GetByID
	got, err := userRepo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("get user by id: %v", err)
	}
	if got.Email != "user@example.com" {
		t.Errorf("email = %q, want %q", got.Email, "user@example.com")
	}
	if got.CustomerID == nil || *got.CustomerID != cust.ID {
		t.Errorf("customer_id = %v, want %d", got.CustomerID, cust.ID)
	}

	// GetByEmail
	got, err = userRepo.GetByEmail(ctx, "user@example.com")
	if err != nil {
		t.Fatalf("get user by email: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("id = %d, want %d", got.ID, u.ID)
	}

	// GetByEmail - not found
	got, err = userRepo.GetByEmail(ctx, "nonexistent@example.com")
	if err != nil {
		t.Fatalf("get nonexistent user: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent email")
	}

	// Update
	u.Email = "updated@example.com"
	if err := userRepo.Update(ctx, u); err != nil {
		t.Fatalf("update user: %v", err)
	}
	got, _ = userRepo.GetByID(ctx, u.ID)
	if got.Email != "updated@example.com" {
		t.Errorf("updated email = %q, want %q", got.Email, "updated@example.com")
	}

	// ListByCustomerID
	list, err := userRepo.ListByCustomerID(ctx, cust.ID)
	if err != nil {
		t.Fatalf("list by customer: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("list len = %d, want 1", len(list))
	}

	// Delete
	if err := userRepo.Delete(ctx, u.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	got, _ = userRepo.GetByID(ctx, u.ID)
	if got != nil {
		t.Error("expected nil after delete")
	}
}

// --- ZoneAssignment tests ---

func TestZoneAssignment(t *testing.T) {
	db := setupTestDB(t)
	zoneRepo := sqlite.NewZoneAssignmentRepo(db)
	custRepo := sqlite.NewCustomerRepo(db)
	ctx := context.Background()

	cust := &models.Customer{Name: "Zone Co"}
	custRepo.Create(ctx, cust)

	// Assign
	a := &models.ZoneAssignment{
		CustomerID: cust.ID,
		ZoneID:     "zone-123",
		ZoneName:   "example.com",
	}
	if err := zoneRepo.Assign(ctx, a); err != nil {
		t.Fatalf("assign zone: %v", err)
	}
	if a.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	// IsZoneAssigned
	assigned, err := zoneRepo.IsZoneAssigned(ctx, "zone-123")
	if err != nil {
		t.Fatalf("is zone assigned: %v", err)
	}
	if !assigned {
		t.Error("expected zone to be assigned")
	}

	// IsZoneAssigned - not assigned
	assigned, _ = zoneRepo.IsZoneAssigned(ctx, "zone-999")
	if assigned {
		t.Error("expected zone not to be assigned")
	}

	// GetCustomerForZone
	got, err := zoneRepo.GetCustomerForZone(ctx, "zone-123")
	if err != nil {
		t.Fatalf("get customer for zone: %v", err)
	}
	if got == nil || got.ID != cust.ID {
		t.Errorf("customer for zone = %v, want id %d", got, cust.ID)
	}

	// ListByCustomerID
	list, err := zoneRepo.ListByCustomerID(ctx, cust.ID)
	if err != nil {
		t.Fatalf("list by customer: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("list len = %d, want 1", len(list))
	}

	// ListAll
	all, err := zoneRepo.ListAll(ctx)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("list all len = %d, want 1", len(all))
	}

	// Unassign
	if err := zoneRepo.Unassign(ctx, cust.ID, "zone-123"); err != nil {
		t.Fatalf("unassign: %v", err)
	}
	assigned, _ = zoneRepo.IsZoneAssigned(ctx, "zone-123")
	if assigned {
		t.Error("expected zone to be unassigned")
	}
}

// --- Session tests ---

func TestSessionCRUD(t *testing.T) {
	db := setupTestDB(t)
	sessRepo := sqlite.NewSessionRepo(db)
	userRepo := sqlite.NewUserRepo(db)
	ctx := context.Background()

	// Create a user first.
	u := &models.User{Email: "sess@example.com", PasswordHash: "hash", Role: models.RoleCustomer}
	userRepo.Create(ctx, u)

	// Create session
	s := &models.Session{
		ID:        "test-session-token",
		UserID:    u.ID,
		ExpiresAt: time.Now().Add(time.Hour).UTC(),
		CreatedAt: time.Now().UTC(),
	}
	if err := sessRepo.Create(ctx, s); err != nil {
		t.Fatalf("create session: %v", err)
	}

	// GetByID
	got, err := sessRepo.GetByID(ctx, "test-session-token")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got == nil {
		t.Fatal("expected session, got nil")
	}
	if got.UserID != u.ID {
		t.Errorf("user_id = %d, want %d", got.UserID, u.ID)
	}

	// GetByID - not found
	got, _ = sessRepo.GetByID(ctx, "nonexistent")
	if got != nil {
		t.Error("expected nil for nonexistent session")
	}

	// Delete
	if err := sessRepo.Delete(ctx, "test-session-token"); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	got, _ = sessRepo.GetByID(ctx, "test-session-token")
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestSessionDeleteByUserID(t *testing.T) {
	db := setupTestDB(t)
	sessRepo := sqlite.NewSessionRepo(db)
	userRepo := sqlite.NewUserRepo(db)
	ctx := context.Background()

	u := &models.User{Email: "multi@example.com", PasswordHash: "hash", Role: models.RoleCustomer}
	userRepo.Create(ctx, u)

	// Create two sessions.
	for _, id := range []string{"s1", "s2"} {
		sessRepo.Create(ctx, &models.Session{
			ID:        id,
			UserID:    u.ID,
			ExpiresAt: time.Now().Add(time.Hour).UTC(),
			CreatedAt: time.Now().UTC(),
		})
	}

	if err := sessRepo.DeleteByUserID(ctx, u.ID); err != nil {
		t.Fatalf("delete by user id: %v", err)
	}

	for _, id := range []string{"s1", "s2"} {
		got, _ := sessRepo.GetByID(ctx, id)
		if got != nil {
			t.Errorf("session %s should have been deleted", id)
		}
	}
}

func TestSessionDeleteExpired(t *testing.T) {
	db := setupTestDB(t)
	sessRepo := sqlite.NewSessionRepo(db)
	userRepo := sqlite.NewUserRepo(db)
	ctx := context.Background()

	u := &models.User{Email: "expire@example.com", PasswordHash: "hash", Role: models.RoleCustomer}
	userRepo.Create(ctx, u)

	// Create an expired session and a valid one.
	sessRepo.Create(ctx, &models.Session{
		ID:        "expired",
		UserID:    u.ID,
		ExpiresAt: time.Now().Add(-time.Hour).UTC(),
		CreatedAt: time.Now().UTC(),
	})
	sessRepo.Create(ctx, &models.Session{
		ID:        "valid",
		UserID:    u.ID,
		ExpiresAt: time.Now().Add(time.Hour).UTC(),
		CreatedAt: time.Now().UTC(),
	})

	if err := sessRepo.DeleteExpired(ctx); err != nil {
		t.Fatalf("delete expired: %v", err)
	}

	got, _ := sessRepo.GetByID(ctx, "expired")
	if got != nil {
		t.Error("expired session should have been deleted")
	}

	got, _ = sessRepo.GetByID(ctx, "valid")
	if got == nil {
		t.Error("valid session should still exist")
	}
}

// --- Migration tests ---

func TestMigrationsIdempotent(t *testing.T) {
	db := setupTestDB(t)
	// Running migrations again should be a no-op.
	if err := sqlite.RunMigrations(db, migrations.SQLiteFS()); err != nil {
		t.Fatalf("re-running migrations: %v", err)
	}
}

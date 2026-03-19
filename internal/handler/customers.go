package handler

import (
	"log/slog"
	"net/http"
	"slices"
	"strconv"

	"zonemeister/internal/auth"
	"zonemeister/internal/middleware"
	"zonemeister/internal/models"
	"zonemeister/internal/netnod"
	"zonemeister/internal/repository"
	"zonemeister/internal/templates"

	"github.com/go-chi/chi/v5"
)

// CustomerFormData is passed to the customer form template.
type CustomerFormData struct {
	Customer     *models.Customer
	IsEdit       bool
	TSIGKeys     []netnod.TSIGKey
	SelectedKeys map[string]bool
}

// CustomerDetailData is passed to the customer detail template.
type CustomerDetailData struct {
	Customer      *models.Customer
	Users         []*models.User
	TSIGKeys      []string
	AvailableKeys []netnod.TSIGKey
}

// CustomerHandler handles customer management HTTP requests.
type CustomerHandler struct {
	customerRepo repository.CustomerRepository
	userRepo     repository.UserRepository
	tsigClient   *netnod.TSIGClient
	tsigKeyRepo  repository.CustomerTSIGKeyRepository
	renderer     *templates.Renderer
}

// NewCustomerHandler creates a new CustomerHandler.
func NewCustomerHandler(
	customerRepo repository.CustomerRepository,
	userRepo repository.UserRepository,
	tsigClient *netnod.TSIGClient,
	tsigKeyRepo repository.CustomerTSIGKeyRepository,
	renderer *templates.Renderer,
) *CustomerHandler {
	return &CustomerHandler{
		customerRepo: customerRepo,
		userRepo:     userRepo,
		tsigClient:   tsigClient,
		tsigKeyRepo:  tsigKeyRepo,
		renderer:     renderer,
	}
}

// List renders a list of all customers.
func (h *CustomerHandler) List(w http.ResponseWriter, r *http.Request) {
	customers, err := h.customerRepo.List(r.Context())
	if err != nil {
		slog.Error("list customers", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := h.renderer.Render(w, r, w, "customers-list", customers); err != nil {
		slog.Error("render customers list", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Show renders the customer detail page with TSIG key management.
func (h *CustomerHandler) Show(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	customer, err := h.customerRepo.GetByID(ctx, id)
	if err != nil {
		slog.Error("get customer", "id", id, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if customer == nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	users, err := h.userRepo.ListByCustomerID(ctx, id)
	if err != nil {
		slog.Error("list users for customer", "id", id, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	tsigKeys, err := h.tsigKeyRepo.ListByCustomerID(ctx, id)
	if err != nil {
		slog.Error("list tsig keys for customer", "id", id, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Fetch available keys from API and filter out already selected ones.
	allKeys, err := h.tsigClient.ListKeys(ctx)
	if err != nil {
		slog.Error("list tsig keys from api", "error", err)
		// Continue without available keys — show page with current keys only.
		allKeys = nil
	}

	selectedSet := make(map[string]bool, len(tsigKeys))
	for _, k := range tsigKeys {
		selectedSet[k] = true
	}

	var availableKeys []netnod.TSIGKey
	for _, k := range allKeys {
		if !selectedSet[k.Name] {
			availableKeys = append(availableKeys, k)
		}
	}

	data := CustomerDetailData{
		Customer:      customer,
		Users:         users,
		TSIGKeys:      tsigKeys,
		AvailableKeys: availableKeys,
	}
	if err := h.renderer.Render(w, r, w, "customers-detail", data); err != nil {
		slog.Error("render customer detail", "id", id, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// New renders the create customer form.
func (h *CustomerHandler) New(w http.ResponseWriter, r *http.Request) {
	tsigKeys, err := h.tsigClient.ListKeys(r.Context())
	if err != nil {
		slog.Error("list tsig keys from api", "error", err)
		tsigKeys = nil
	}

	data := CustomerFormData{
		Customer:     &models.Customer{},
		IsEdit:       false,
		TSIGKeys:     tsigKeys,
		SelectedKeys: map[string]bool{},
	}
	if err := h.renderer.Render(w, r, w, "customers-form", data); err != nil {
		slog.Error("render new customer form", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Create handles submission of the create customer form.
func (h *CustomerHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	customer := &models.Customer{
		Name:  r.FormValue("name"),
		Email: r.FormValue("customer_email"),
		Notes: r.FormValue("notes"),
	}

	if err := h.customerRepo.Create(ctx, customer); err != nil {
		slog.Error("create customer", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Create the first user account for this customer.
	hash, err := auth.HashPassword(r.FormValue("password"))
	if err != nil {
		slog.Error("hash password for new customer user", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user := &models.User{
		Email:        r.FormValue("user_email"),
		PasswordHash: hash,
		Role:         models.RoleCustomer,
		CustomerID:   &customer.ID,
	}

	if err := h.userRepo.Create(ctx, user); err != nil {
		slog.Error("create user for customer", "customer_id", customer.ID, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Save selected TSIG keys.
	if err := r.ParseForm(); err == nil {
		if tsigKeys := r.Form["tsig_keys"]; len(tsigKeys) > 0 {
			if err := h.tsigKeyRepo.SetForCustomer(ctx, customer.ID, tsigKeys); err != nil {
				slog.Error("set tsig keys for customer", "customer_id", customer.ID, "error", err)
			}
		}
	}

	middleware.SetFlash(w, "Customer created.")
	http.Redirect(w, r, "/admin/customers", http.StatusFound)
}

// Edit renders the edit customer form.
func (h *CustomerHandler) Edit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	customer, err := h.customerRepo.GetByID(ctx, id)
	if err != nil {
		slog.Error("get customer for edit", "id", id, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if customer == nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	tsigKeys, err := h.tsigClient.ListKeys(ctx)
	if err != nil {
		slog.Error("list tsig keys from api", "error", err)
		tsigKeys = nil
	}

	currentKeys, err := h.tsigKeyRepo.ListByCustomerID(ctx, id)
	if err != nil {
		slog.Error("list tsig keys for customer", "id", id, "error", err)
		currentKeys = nil
	}

	selectedKeys := make(map[string]bool, len(currentKeys))
	for _, k := range currentKeys {
		selectedKeys[k] = true
	}

	data := CustomerFormData{
		Customer:     customer,
		IsEdit:       true,
		TSIGKeys:     tsigKeys,
		SelectedKeys: selectedKeys,
	}
	if err := h.renderer.Render(w, r, w, "customers-form", data); err != nil {
		slog.Error("render edit customer form", "id", id, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Update handles submission of the edit customer form.
func (h *CustomerHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	customer, err := h.customerRepo.GetByID(ctx, id)
	if err != nil {
		slog.Error("get customer for update", "id", id, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if customer == nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	customer.Name = r.FormValue("name")
	customer.Email = r.FormValue("customer_email")
	customer.Notes = r.FormValue("notes")

	if err := h.customerRepo.Update(ctx, customer); err != nil {
		slog.Error("update customer", "id", id, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Update TSIG keys.
	if err := r.ParseForm(); err == nil {
		tsigKeys := r.Form["tsig_keys"]
		if err := h.tsigKeyRepo.SetForCustomer(ctx, id, tsigKeys); err != nil {
			slog.Error("set tsig keys for customer", "customer_id", id, "error", err)
		}
	}

	middleware.SetFlash(w, "Customer updated.")
	http.Redirect(w, r, "/admin/customers", http.StatusFound)
}

// AddTSIG adds a TSIG key to a customer.
func (h *CustomerHandler) AddTSIG(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	keyName := r.FormValue("tsig_key_name")
	if keyName == "" {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	currentKeys, err := h.tsigKeyRepo.ListByCustomerID(ctx, id)
	if err != nil {
		slog.Error("list tsig keys for customer", "id", id, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Add the new key if not already present.
	if !slices.Contains(currentKeys, keyName) {
		currentKeys = append(currentKeys, keyName)
	}

	if err := h.tsigKeyRepo.SetForCustomer(ctx, id, currentKeys); err != nil {
		slog.Error("add tsig key for customer", "customer_id", id, "key", keyName, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	middleware.SetFlash(w, "TSIG key added.")
	http.Redirect(w, r, "/admin/customers/"+strconv.FormatInt(id, 10), http.StatusFound)
}

// RemoveTSIG removes a TSIG key from a customer.
func (h *CustomerHandler) RemoveTSIG(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	keyName := r.FormValue("tsig_key_name")
	if keyName == "" {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	currentKeys, err := h.tsigKeyRepo.ListByCustomerID(ctx, id)
	if err != nil {
		slog.Error("list tsig keys for customer", "id", id, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Remove the key.
	var newKeys []string
	for _, k := range currentKeys {
		if k != keyName {
			newKeys = append(newKeys, k)
		}
	}

	if err := h.tsigKeyRepo.SetForCustomer(ctx, id, newKeys); err != nil {
		slog.Error("remove tsig key for customer", "customer_id", id, "key", keyName, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	middleware.SetFlash(w, "TSIG key removed.")
	http.Redirect(w, r, "/admin/customers/"+strconv.FormatInt(id, 10), http.StatusFound)
}

// AddUser adds a new user to a customer.
func (h *CustomerHandler) AddUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")
	if email == "" || password == "" {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		slog.Error("hash password for new user", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user := &models.User{
		Email:        email,
		PasswordHash: hash,
		Role:         models.RoleCustomer,
		CustomerID:   &id,
	}

	if err := h.userRepo.Create(ctx, user); err != nil {
		slog.Error("create user for customer", "customer_id", id, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	middleware.SetFlash(w, "User added.")
	http.Redirect(w, r, "/admin/customers/"+strconv.FormatInt(id, 10), http.StatusFound)
}

// DeleteUser removes a user from a customer.
func (h *CustomerHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	userID, err := strconv.ParseInt(r.FormValue("user_id"), 10, 64)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if err := h.userRepo.Delete(ctx, userID); err != nil {
		slog.Error("delete user", "user_id", userID, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	middleware.SetFlash(w, "User deleted.")
	http.Redirect(w, r, "/admin/customers/"+strconv.FormatInt(id, 10), http.StatusFound)
}

// ResetUserPassword resets a user's password.
func (h *CustomerHandler) ResetUserPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	userID, err := strconv.ParseInt(r.FormValue("user_id"), 10, 64)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	password := r.FormValue("password")
	if password == "" {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	user, err := h.userRepo.GetByID(ctx, userID)
	if err != nil {
		slog.Error("get user for password reset", "user_id", userID, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if user == nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		slog.Error("hash password for reset", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user.PasswordHash = hash
	if err := h.userRepo.Update(ctx, user); err != nil {
		slog.Error("update user password", "user_id", userID, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	middleware.SetFlash(w, "Password reset.")
	http.Redirect(w, r, "/admin/customers/"+strconv.FormatInt(id, 10), http.StatusFound)
}

// Delete handles deletion of a customer.
func (h *CustomerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if err := h.customerRepo.Delete(r.Context(), id); err != nil {
		slog.Error("delete customer", "id", id, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	middleware.SetFlash(w, "Customer deleted.")
	http.Redirect(w, r, "/admin/customers", http.StatusFound)
}

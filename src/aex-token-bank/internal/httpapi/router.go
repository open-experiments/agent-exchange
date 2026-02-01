package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/parlakisik/agent-exchange/aex-token-bank/internal/ap2"
	"github.com/parlakisik/agent-exchange/aex-token-bank/internal/model"
	"github.com/parlakisik/agent-exchange/aex-token-bank/internal/service"
	"github.com/parlakisik/agent-exchange/aex-token-bank/internal/store"
)

// Router handles HTTP requests for the token bank API
type Router struct {
	svc         *service.TokenService
	mux         *http.ServeMux
	ap2Handler  *ap2.Handler
	ap2Provider *ap2.TokenPaymentProvider
}

// ServiceTransferAdapter adapts TokenService to ap2.TransferHandler interface
type ServiceTransferAdapter struct {
	svc *service.TokenService
}

// Transfer implements ap2.TransferHandler
func (a *ServiceTransferAdapter) Transfer(fromAgentID, toAgentID string, amount float64, reference, description string) (string, error) {
	tx, err := a.svc.Transfer(&model.TransferRequest{
		FromAgentID: fromAgentID,
		ToAgentID:   toAgentID,
		Amount:      amount,
		Reference:   reference,
		Description: description,
	})
	if err != nil {
		return "", err
	}
	return tx.ID, nil
}

// GetBalance implements ap2.TransferHandler
func (a *ServiceTransferAdapter) GetBalance(agentID string) (float64, error) {
	resp, err := a.svc.GetBalance(agentID)
	if err != nil {
		return 0, err
	}
	return resp.Balance, nil
}

// NewRouter creates a new HTTP router
func NewRouter(svc *service.TokenService) *Router {
	// Create AP2 provider with service adapter
	adapter := &ServiceTransferAdapter{svc: svc}
	ap2Provider := ap2.NewTokenPaymentProvider(adapter)
	ap2Handler := ap2.NewHandler(ap2Provider)

	r := &Router{
		svc:         svc,
		mux:         http.NewServeMux(),
		ap2Handler:  ap2Handler,
		ap2Provider: ap2Provider,
	}

	r.setupRoutes()
	return r
}

// GetAP2Provider returns the AP2 payment provider for external use
func (r *Router) GetAP2Provider() *ap2.TokenPaymentProvider {
	return r.ap2Provider
}

func (r *Router) setupRoutes() {
	// Health check
	r.mux.HandleFunc("GET /health", r.healthCheck)

	// Treasury endpoint (public - shows supply info)
	r.mux.HandleFunc("GET /treasury", r.getTreasury)

	// Authenticated "me" endpoints - agent accesses own wallet
	r.mux.HandleFunc("GET /wallets/me", r.getMyWallet)
	r.mux.HandleFunc("GET /wallets/me/balance", r.getMyBalance)
	r.mux.HandleFunc("GET /wallets/me/history", r.getMyTransactionHistory)

	// Wallet endpoints (legacy - for backwards compatibility)
	r.mux.HandleFunc("POST /wallets", r.createWallet)
	r.mux.HandleFunc("GET /wallets", r.listWallets)
	r.mux.HandleFunc("GET /wallets/{agent_id}", r.getWallet)
	r.mux.HandleFunc("GET /wallets/{agent_id}/balance", r.getBalance)
	r.mux.HandleFunc("POST /wallets/{agent_id}/deposit", r.deposit)
	r.mux.HandleFunc("POST /wallets/{agent_id}/withdraw", r.withdraw)
	r.mux.HandleFunc("GET /wallets/{agent_id}/history", r.getTransactionHistory)

	// Transfer endpoint
	r.mux.HandleFunc("POST /transfers", r.transfer)

	// AP2 Payment Protocol endpoints
	r.ap2Handler.RegisterRoutes(r.mux)
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if req.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	r.mux.ServeHTTP(w, req)
}

func (r *Router) healthCheck(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"service": "aex-token-bank",
	})
}

func (r *Router) createWallet(w http.ResponseWriter, req *http.Request) {
	var createReq model.CreateWalletRequest
	if err := json.NewDecoder(req.Body).Decode(&createReq); err != nil {
		r.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if createReq.AgentID == "" {
		r.writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	wallet, err := r.svc.CreateWallet(&createReq)
	if err != nil {
		if err == store.ErrWalletAlreadyExists {
			r.writeError(w, http.StatusConflict, err.Error())
			return
		}
		slog.Error("failed to create wallet", "error", err)
		r.writeError(w, http.StatusInternalServerError, "failed to create wallet")
		return
	}

	slog.Info("wallet created", "agent_id", wallet.AgentID, "balance", wallet.Balance)
	r.writeJSON(w, http.StatusCreated, wallet)
}

func (r *Router) listWallets(w http.ResponseWriter, req *http.Request) {
	response, err := r.svc.GetAllWallets()
	if err != nil {
		slog.Error("failed to list wallets", "error", err)
		r.writeError(w, http.StatusInternalServerError, "failed to list wallets")
		return
	}

	r.writeJSON(w, http.StatusOK, response)
}

func (r *Router) getWallet(w http.ResponseWriter, req *http.Request) {
	agentID := r.extractAgentID(req)
	if agentID == "" {
		r.writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	wallet, err := r.svc.GetWallet(agentID)
	if err != nil {
		if err == store.ErrWalletNotFound {
			r.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		slog.Error("failed to get wallet", "error", err)
		r.writeError(w, http.StatusInternalServerError, "failed to get wallet")
		return
	}

	r.writeJSON(w, http.StatusOK, wallet)
}

func (r *Router) getBalance(w http.ResponseWriter, req *http.Request) {
	agentID := r.extractAgentID(req)
	if agentID == "" {
		r.writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	response, err := r.svc.GetBalance(agentID)
	if err != nil {
		if err == store.ErrWalletNotFound {
			r.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		slog.Error("failed to get balance", "error", err)
		r.writeError(w, http.StatusInternalServerError, "failed to get balance")
		return
	}

	r.writeJSON(w, http.StatusOK, response)
}

func (r *Router) deposit(w http.ResponseWriter, req *http.Request) {
	agentID := r.extractAgentID(req)
	if agentID == "" {
		r.writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	var depositReq model.DepositRequest
	if err := json.NewDecoder(req.Body).Decode(&depositReq); err != nil {
		r.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if depositReq.Amount <= 0 {
		r.writeError(w, http.StatusBadRequest, "amount must be positive")
		return
	}

	tx, err := r.svc.Deposit(agentID, &depositReq)
	if err != nil {
		if err == store.ErrWalletNotFound {
			r.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		slog.Error("failed to deposit", "error", err)
		r.writeError(w, http.StatusInternalServerError, "failed to deposit")
		return
	}

	slog.Info("deposit completed", "agent_id", agentID, "amount", depositReq.Amount)
	r.writeJSON(w, http.StatusOK, tx)
}

func (r *Router) withdraw(w http.ResponseWriter, req *http.Request) {
	agentID := r.extractAgentID(req)
	if agentID == "" {
		r.writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	var withdrawReq model.WithdrawRequest
	if err := json.NewDecoder(req.Body).Decode(&withdrawReq); err != nil {
		r.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if withdrawReq.Amount <= 0 {
		r.writeError(w, http.StatusBadRequest, "amount must be positive")
		return
	}

	tx, err := r.svc.Withdraw(agentID, &withdrawReq)
	if err != nil {
		if err == store.ErrWalletNotFound {
			r.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if err == store.ErrInsufficientBalance {
			r.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		slog.Error("failed to withdraw", "error", err)
		r.writeError(w, http.StatusInternalServerError, "failed to withdraw")
		return
	}

	slog.Info("withdrawal completed", "agent_id", agentID, "amount", withdrawReq.Amount)
	r.writeJSON(w, http.StatusOK, tx)
}

func (r *Router) transfer(w http.ResponseWriter, req *http.Request) {
	var transferReq model.TransferRequest
	if err := json.NewDecoder(req.Body).Decode(&transferReq); err != nil {
		r.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if transferReq.FromAgentID == "" || transferReq.ToAgentID == "" {
		r.writeError(w, http.StatusBadRequest, "from_agent_id and to_agent_id are required")
		return
	}

	if transferReq.Amount <= 0 {
		r.writeError(w, http.StatusBadRequest, "amount must be positive")
		return
	}

	tx, err := r.svc.Transfer(&transferReq)
	if err != nil {
		if err == store.ErrInsufficientBalance {
			r.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if strings.Contains(err.Error(), "not found") {
			r.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		slog.Error("failed to transfer", "error", err)
		r.writeError(w, http.StatusInternalServerError, "failed to transfer")
		return
	}

	slog.Info("transfer completed",
		"from", transferReq.FromAgentID,
		"to", transferReq.ToAgentID,
		"amount", transferReq.Amount,
	)
	r.writeJSON(w, http.StatusOK, tx)
}

func (r *Router) getTransactionHistory(w http.ResponseWriter, req *http.Request) {
	agentID := r.extractAgentID(req)
	if agentID == "" {
		r.writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	response, err := r.svc.GetTransactionHistory(agentID)
	if err != nil {
		if err == store.ErrWalletNotFound {
			r.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		slog.Error("failed to get transaction history", "error", err)
		r.writeError(w, http.StatusInternalServerError, "failed to get transaction history")
		return
	}

	r.writeJSON(w, http.StatusOK, response)
}

func (r *Router) extractAgentID(req *http.Request) string {
	return req.PathValue("agent_id")
}

func (r *Router) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (r *Router) writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(model.ErrorResponse{
		Error:   http.StatusText(status),
		Message: message,
	})
}

// ===== Phase 7: Secure Banking Endpoints =====

// getTreasury returns the current treasury state (public endpoint)
func (r *Router) getTreasury(w http.ResponseWriter, req *http.Request) {
	response, err := r.svc.GetTreasury()
	if err != nil {
		// If treasury not initialized, return zeros
		if err == store.ErrTreasuryNotInitialized {
			r.writeJSON(w, http.StatusOK, model.TreasuryResponse{
				TotalSupply: 0,
				Allocated:   0,
				Available:   0,
				TokenType:   "AEX",
			})
			return
		}
		slog.Error("failed to get treasury", "error", err)
		r.writeError(w, http.StatusInternalServerError, "failed to get treasury")
		return
	}

	r.writeJSON(w, http.StatusOK, response)
}

// getMyWallet returns the authenticated agent's wallet
func (r *Router) getMyWallet(w http.ResponseWriter, req *http.Request) {
	agentID := r.getAuthenticatedAgentID(req)
	if agentID == "" {
		r.writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	wallet, err := r.svc.GetWallet(agentID)
	if err != nil {
		if err == store.ErrWalletNotFound {
			r.writeError(w, http.StatusNotFound, "wallet not found")
			return
		}
		slog.Error("failed to get wallet", "error", err)
		r.writeError(w, http.StatusInternalServerError, "failed to get wallet")
		return
	}

	r.writeJSON(w, http.StatusOK, wallet)
}

// getMyBalance returns the authenticated agent's balance
func (r *Router) getMyBalance(w http.ResponseWriter, req *http.Request) {
	agentID := r.getAuthenticatedAgentID(req)
	if agentID == "" {
		r.writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	response, err := r.svc.GetBalance(agentID)
	if err != nil {
		if err == store.ErrWalletNotFound {
			r.writeError(w, http.StatusNotFound, "wallet not found")
			return
		}
		slog.Error("failed to get balance", "error", err)
		r.writeError(w, http.StatusInternalServerError, "failed to get balance")
		return
	}

	r.writeJSON(w, http.StatusOK, response)
}

// getMyTransactionHistory returns the authenticated agent's transaction history
func (r *Router) getMyTransactionHistory(w http.ResponseWriter, req *http.Request) {
	agentID := r.getAuthenticatedAgentID(req)
	if agentID == "" {
		r.writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	response, err := r.svc.GetTransactionHistory(agentID)
	if err != nil {
		if err == store.ErrWalletNotFound {
			r.writeError(w, http.StatusNotFound, "wallet not found")
			return
		}
		slog.Error("failed to get transaction history", "error", err)
		r.writeError(w, http.StatusInternalServerError, "failed to get transaction history")
		return
	}

	r.writeJSON(w, http.StatusOK, response)
}

// getAuthenticatedAgentID extracts and validates the agent from the Bearer token
func (r *Router) getAuthenticatedAgentID(req *http.Request) string {
	// First check if already set in context (by middleware)
	if agentID := GetAuthenticatedAgentID(req); agentID != "" {
		return agentID
	}

	// Otherwise, try to extract from Authorization header directly
	authHeader := req.Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		return ""
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		return ""
	}

	tokenHash := SHA256Hex(token)
	agentID, err := r.svc.GetAgentIDByTokenHash(tokenHash)
	if err != nil {
		return ""
	}

	return agentID
}

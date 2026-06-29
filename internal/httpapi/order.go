package httpapi

import (
	"net/http"

	"github.com/yourusername/orderflow/internal/domain"
	"github.com/yourusername/orderflow/internal/service"
)

func (s *Server) handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []struct {
			ProductID int64 `json:"product_id"`
			Quantity  int   `json:"quantity"`
		} `json:"items"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}

	lines := make([]service.OrderLineInput, 0, len(body.Items))
	for _, it := range body.Items {
		lines = append(lines, service.OrderLineInput{
			ProductID: it.ProductID,
			Quantity:  it.Quantity,
		})
	}

	userID := userIDFromContext(r.Context())
	order, err := s.orders.Create(r.Context(), userID, service.CreateOrderInput{Items: lines})
	if err != nil {
		writeError(w, s.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

func (s *Server) handleListOrders(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	userID := userIDFromContext(r.Context())

	orders, err := s.orders.ListByUser(r.Context(), userID, limit, offset)
	if err != nil {
		writeError(w, s.logger, err)
		return
	}
	if orders == nil {
		orders = []domain.Order{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": orders})
}

func (s *Server) handleGetOrder(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid order id"})
		return
	}

	userID := userIDFromContext(r.Context())
	role := roleFromContext(r.Context())

	order, err := s.orders.Get(r.Context(), userID, role, id)
	if err != nil {
		writeError(w, s.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (s *Server) handleUpdateOrderStatus(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid order id"})
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}

	order, err := s.orders.UpdateStatus(r.Context(), id, domain.OrderStatus(body.Status))
	if err != nil {
		writeError(w, s.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

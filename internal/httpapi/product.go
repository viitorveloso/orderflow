package httpapi

import (
	"net/http"

	"github.com/yourusername/orderflow/internal/domain"
	"github.com/yourusername/orderflow/internal/service"
)

// productBody is the JSON shape for creating and updating products.
type productBody struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	PriceCents  int64  `json:"price_cents"`
	Stock       int    `json:"stock"`
}

func (b productBody) toInput() service.ProductInput {
	return service.ProductInput{
		Name:        b.Name,
		Description: b.Description,
		PriceCents:  b.PriceCents,
		Stock:       b.Stock,
	}
}

func (s *Server) handleListProducts(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	products, err := s.products.List(r.Context(), limit, offset)
	if err != nil {
		writeError(w, s.logger, err)
		return
	}
	if products == nil {
		products = []domain.Product{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"products": products})
}

func (s *Server) handleGetProduct(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid product id"})
		return
	}
	product, err := s.products.Get(r.Context(), id)
	if err != nil {
		writeError(w, s.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, product)
}

func (s *Server) handleCreateProduct(w http.ResponseWriter, r *http.Request) {
	var body productBody
	if err := decodeJSON(w, r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}
	product, err := s.products.Create(r.Context(), body.toInput())
	if err != nil {
		writeError(w, s.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, product)
}

func (s *Server) handleUpdateProduct(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid product id"})
		return
	}
	var body productBody
	if err := decodeJSON(w, r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}
	product, err := s.products.Update(r.Context(), id, body.toInput())
	if err != nil {
		writeError(w, s.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, product)
}

func (s *Server) handleDeleteProduct(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid product id"})
		return
	}
	if err := s.products.Delete(r.Context(), id); err != nil {
		writeError(w, s.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

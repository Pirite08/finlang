// Package transferhttp exposes the local test ledger over HTTP.
package transferhttp

import (
	"context"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/Pirite08/finlang/internal/transfer"
)

const maxRequestBytes = 8 << 10

type transferStore interface {
	Transfer(context.Context, transfer.Request) (transfer.Result, error)
}

type requestBody struct {
	ID       *string `json:"request_id"`
	From     *string `json:"from_account"`
	To       *string `json:"to_account"`
	Currency *string `json:"currency"`
	Amount   *int64  `json:"amount_minor"`
}

// New serves transfer requests against the supplied ledger.
func New(store transferStore) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/transfers", func(w http.ResponseWriter, r *http.Request) {
		handleTransfer(w, r, store)
	})
	return mux
}

func handleTransfer(w http.ResponseWriter, r *http.Request, store transferStore) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	charset := params["charset"]
	if err != nil || mediaType != "application/json" || (charset != "" && !strings.EqualFold(charset, "utf-8")) {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			writeError(w, http.StatusRequestEntityTooLarge, "request_too_large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid_request")
		}
		return
	}
	var input requestBody
	err = jsonv2.Unmarshal(body, &input, jsonv2.RejectUnknownMembers(true))
	if err != nil || input.ID == nil || input.From == nil || input.To == nil || input.Currency == nil || input.Amount == nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	result, err := store.Transfer(r.Context(), transfer.Request{
		ID:       *input.ID,
		From:     *input.From,
		To:       *input.To,
		Currency: *input.Currency,
		Amount:   *input.Amount,
	})
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, result)
	case errors.Is(err, transfer.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, transfer.ErrCurrency):
		writeError(w, http.StatusBadRequest, "currency_mismatch")
	case errors.Is(err, transfer.ErrNotFound):
		writeError(w, http.StatusNotFound, "account_not_found")
	case errors.Is(err, transfer.ErrConflict):
		writeError(w, http.StatusConflict, "request_conflict")
	case errors.Is(err, transfer.ErrInsufficientFunds):
		writeError(w, http.StatusConflict, "insufficient_funds")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error")
	}
}

func writeError(w http.ResponseWriter, status int, code string) {
	response := struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}{}
	response.Error.Code = code
	writeJSON(w, status, response)
}

func writeJSON(w http.ResponseWriter, status int, result any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(result)
}

package cli

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/Pirite08/finlang/internal/contract"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/eval"
	"github.com/Pirite08/finlang/internal/transfer"
)

func newRuleHandler(app servedApp) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")

		host := r.Host
		if name, _, err := net.SplitHostPort(host); err == nil {
			host = name
		}
		// Prevent a hostname that resolves to loopback from reaching the local API.
		if host != "127.0.0.1" && !strings.EqualFold(host, "localhost") {
			writeServeError(w, http.StatusForbidden, contract.Spec{}, "E_HTTP_HOST", "local host required")
			return
		}
		path := r.URL.EscapedPath()
		if path == "/" && len(app.preview) != 0 {
			servePreview(w, r, app.preview)
			return
		}
		route, ok := app.routes[path]
		if !ok {
			writeServeError(w, http.StatusNotFound, contract.Spec{}, "E_HTTP_ROUTE", "route not found")
			return
		}
		allow := http.MethodPost
		if route.transfer {
			allow = "POST, OPTIONS"
			if r.Method == http.MethodOptions {
				w.Header().Set("Allow", allow)
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", allow)
			writeServeError(w, http.StatusMethodNotAllowed, route.spec, "E_HTTP_METHOD", "use POST")
			return
		}
		serveRuleRequest(w, r, route, app.ledger)
	})
}

func servePreview(w http.ResponseWriter, r *http.Request, page []byte) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeServeError(w, http.StatusMethodNotAllowed, contract.Spec{}, "E_HTTP_METHOD", "use GET or HEAD")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
	w.Header().Set("Content-Length", strconv.Itoa(len(page)))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	_, _ = w.Write(page)
}

func serveRuleRequest(w http.ResponseWriter, r *http.Request, route servedRule, ledger *transfer.Store) {
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	charset := params["charset"]
	if err != nil || mediaType != "application/json" || (charset != "" && !strings.EqualFold(charset, "utf-8")) {
		writeServeError(w, http.StatusUnsupportedMediaType, route.spec, "E_HTTP_CONTENT_TYPE", "use UTF-8 application/json")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(route.limits.MaxSourceBytes)))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeServeError(w, http.StatusRequestEntityTooLarge, route.spec, "E_HTTP_BODY_LIMIT", "request body exceeds byte limit")
		} else {
			writeServeError(w, http.StatusBadRequest, route.spec, "E_CONTRACT_INPUT", "cannot read request body")
		}
		return
	}
	args, err := contract.DecodeInput(bytes.NewReader(body), route.spec, route.limits)
	if err != nil {
		if route.transfer && ledger != nil && serveHistoricalTransferRetry(w, r, route, ledger, body) {
			return
		}
		writeServeError(w, http.StatusBadRequest, route.spec, diagnostic.CodeOf(err), "request does not match the input contract")
		return
	}
	ordered := make([]eval.Value, len(route.inputOrder))
	for i, index := range route.inputOrder {
		ordered[i] = args[index]
	}
	if route.transfer {
		if ledger == nil {
			writeServeError(w, http.StatusInternalServerError, route.spec, "E_INTERNAL", "transfer ledger unavailable")
			return
		}
		servePlannedTransfer(w, r, route, ledger, body, ordered)
		return
	}
	evaluator := eval.New(eval.Options{
		Limits: route.limits, JSONMode: true, RuleOnly: true, CollectionRules: route.spec.SchemaVersion != "1",
	})
	result, err := evaluator.RunRule(route.rule, ordered...)
	if err != nil {
		code := diagnostic.CodeOf(err)
		if code == "E_INTERNAL" {
			writeServeError(w, http.StatusInternalServerError, route.spec, code, "rule execution failed")
		} else {
			writeServeError(w, http.StatusUnprocessableEntity, route.spec, code, "rule could not evaluate the request")
		}
		return
	}
	var output bytes.Buffer
	if err := contract.EncodeResult(&output, route.spec, result); err != nil {
		writeServeError(w, http.StatusInternalServerError, route.spec, diagnostic.CodeOf(err), "rule result does not match the output contract")
		return
	}
	_, _ = output.WriteTo(w)
}

func writeServeError(w http.ResponseWriter, status int, spec contract.Spec, code diagnostic.Code, message string) {
	w.WriteHeader(status)
	_ = contract.EncodeErrorVersion(w, spec.SchemaVersion, spec.Rule, diagnostic.New(code, message))
}

package maintenance

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"time"
)

// Handler authenticates scheduled requests independently of user bearer tokens.
func Handler(secret string, run func(context.Context) error, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		expected := sha256.Sum256([]byte("Bearer " + secret))
		provided := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		if len(secret) < 32 || subtle.ConstantTimeCompare(expected[:], provided[:]) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		if err := run(ctx); err != nil {
			log.Error("maintenance failed")
			http.Error(w, "Maintenance failed", http.StatusInternalServerError)
			return
		}
		log.Info("maintenance completed")
		w.WriteHeader(http.StatusNoContent)
	})
}

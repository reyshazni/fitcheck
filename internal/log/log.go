package log

import (
	"log/slog"
	"os"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Setup initializes structured JSON logging for the application.
// Wires slog through logr so controller-runtime uses the same logger.
func Setup() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := logr.FromSlogHandler(handler)
	ctrl.SetLogger(logger)
	slog.SetDefault(slog.New(handler))
}

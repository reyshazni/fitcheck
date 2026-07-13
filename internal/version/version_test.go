package version_test

import (
	"testing"

	"github.com/reyshazni/fitcheck/internal/version"
)

func TestInfoReturnsStructuredVersion(t *testing.T) {
	t.Parallel()

	info := version.Info()

	if info.Version == "" {
		t.Error("expected Version to have a default value")
	}

	if info.Commit == "" {
		t.Error("expected Commit to have a default value")
	}

	if info.Date == "" {
		t.Error("expected Date to have a default value")
	}
}

func TestInfoStringFormat(t *testing.T) {
	t.Parallel()

	info := version.Info()
	s := info.String()

	if s == "" {
		t.Error("expected String() to return non-empty value")
	}
}

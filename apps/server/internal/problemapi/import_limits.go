package problemapi

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

const importMultipartOverhead = 16 << 20

// importLimits returns the zip and decompressed limits for the current
// request. Settings take precedence; MaxImportFileBytes remains a test/legacy
// fallback.
func (h *handler) importLimits(ctx context.Context) (int64, int64, error) {
	if h.importSettings != nil {
		view, err := h.importSettings.Get(ctx)
		if err != nil {
			return 0, 0, err
		}
		if view != nil && view.MaxZipBytes > 0 && view.MaxUncompressedBytes > 0 {
			return view.MaxZipBytes, view.MaxUncompressedBytes, nil
		}
	}
	zip := h.maxImportFileBytes
	if zip <= 0 {
		zip = problem.MaxHydroZipBytes
	}
	uncompressed := h.maxImportUncompressedBytes
	if uncompressed <= 0 {
		uncompressed = int64(problem.MaxHydroUncompressed)
		if zip*2 > uncompressed {
			uncompressed = zip * 2
		}
	}
	return zip, uncompressed, nil
}

func (h *handler) prepareImportBody(c *gin.Context) (int64, int64, bool) {
	zip, uncompressed, err := h.importLimits(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return 0, 0, false
	}
	maxBody := zip + importMultipartOverhead
	if c.Request.ContentLength > maxBody {
		httpx.Fail(c, httpx.ErrPayloadTooLarge)
		return 0, 0, false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBody)
	return zip, uncompressed, true
}

func (h *handler) importLimitsView(c *gin.Context) {
	zip, uncompressed, err := h.importLimits(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	zipMB := (zip + (1 << 20) - 1) >> 20
	httpx.OK(c, gin.H{
		"maxPackageMb":         zipMB,
		"maxUncompressedMb":    uncompressed >> 20,
		"maxPackageBytes":      zip,
		"maxUncompressedBytes": uncompressed,
	})
}

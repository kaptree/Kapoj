package health

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

// Handler reports the status of the database and Redis.
func Handler(db, redis Pinger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		data := gin.H{"status": "ok", "db": "ok", "redis": "ok"}
		healthy := true
		if err := db.Ping(ctx); err != nil {
			data["db"], healthy = "down", false
		}
		if err := redis.Ping(ctx); err != nil {
			data["redis"], healthy = "down", false
		}
		if !healthy {
			data["status"] = "degraded"
			httpx.FailWithData(c, httpx.ErrUnavailable, data)
			return
		}
		httpx.OK(c, data)
	}
}

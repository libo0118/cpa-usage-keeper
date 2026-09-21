package api

import (
	"context"
	"cpa-usage-keeper/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
)

type KeyPolicyProvider interface {
	GetKeyPolicies(context.Context) (service.KeyPolicyReport, error)
}

func registerKeyPolicyRoutes(router gin.IRoutes, provider KeyPolicyProvider) {
	router.GET("/key-policies", func(c *gin.Context) {
		setNoStoreHeaders(c)
		if provider == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "key_budgets_unavailable"})
			return
		}
		result, err := provider.GetKeyPolicies(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "key_budgets_unavailable", "sync": result.Sync})
			return
		}
		c.JSON(http.StatusOK, result)
	})
}

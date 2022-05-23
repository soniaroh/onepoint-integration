package main

import (
	"github.com/gin-gonic/gin"
)

func main() {
	initEnv()
	initLogger()
	loadPasswords()
	initDB()
	initEmailTemplates()
	initOPAPIConnections()
	initCache()

	if env.Production {
		gin.SetMode(gin.ReleaseMode)
		gin.DisableConsoleColor()
	}

	router := gin.New()

	if env.Production {
		router.Use(GinLogger())
	} else {
		router.Use(gin.Logger())
	}

	router.Use(gin.Recovery())

	registerRoutes(router)

	runScripts()

	router.Run(":8080")
}

func registerRoutes(router *gin.Engine) {
	registerAuthRoutes(router)
	registerAdminRoutes(router)
	registerBgCheckRoutes(router)
	registerCloudPunchRoutes(router)
	registerCompanyIntegrationRoutes(router)
	registerCompanyProductRoutes(router)
	registerCompanyRoutes(router)
	registerEventRoutes(router)
	registerGoogleHireRoutes(router)
	registerIntegrationRoutes(router)
	registerJobsRoutes(router)
	registerOauthTokenRoutes(router)
	registerProductRoutes(router)
}

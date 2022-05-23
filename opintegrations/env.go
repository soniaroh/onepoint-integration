package main

import (
	"context"
	"log"
	"os"
	"time"

	"cloud.google.com/go/logging"
	"github.com/gin-gonic/gin"
)

type Env struct {
	Production bool
}

var (
	env      *Env
	InfoLog  *log.Logger
	ErrorLog *log.Logger
	GinLog   *log.Logger
)

const logFile = "/var/log/opintegrations/log.txt"

func initEnv() {
	runningEnvironment := os.Getenv("ENV")

	isProduction := runningEnvironment == "prod"

	env = &Env{
		Production: isProduction,
	}
}

func initLogger() {
	//for now they always write to same place, just diff prefixes
	infoHandle := os.Stdout

	InfoLog = log.New(infoHandle, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	ErrorLog = log.New(infoHandle, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)
	GinLog = log.New(infoHandle, "", log.Ltime)

	if env.Production {
		ctx := context.Background()

		projectID := "onepoint-integrations"

		client, err := logging.NewClient(ctx, projectID)
		if err != nil {
			log.Fatalf("Failed to create client: %v", err)
		}

		logName := "op-log"

		InfoLog = client.Logger(logName).StandardLogger(logging.Info)
		InfoLog.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

		ErrorLog = client.Logger(logName).StandardLogger(logging.Error)
		ErrorLog.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

		GinLog = client.Logger(logName).StandardLogger(logging.Info)
		GinLog.SetFlags(log.Ltime)
	}
}

func GinLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		end := time.Now()
		latency := end.Sub(start)

		clientIP := c.GetHeader("X-Real-IP")
		method := c.Request.Method
		statusCode := c.Writer.Status()
		var statusColor, methodColor, resetColor string

		if raw != "" {
			path = path + "?" + raw
		}

		GinLog.Printf("[GIN] %v |%s %3d %s| %13v | %15s |%s %-7s %s %s\n",
			end.Format("2006/01/02 - 15:04:05"),
			statusColor, statusCode, resetColor,
			latency,
			clientIP,
			methodColor, method, resetColor,
			path,
		)
	}
}

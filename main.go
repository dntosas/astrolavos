// Main package inits all of Astrolavos components
package main

import (
	"flag"
	"os"

	"github.com/dntosas/astrolavos/internal/config"
	"github.com/dntosas/astrolavos/internal/machinery"

	log "github.com/sirupsen/logrus"
)

var (
	// Version of the tool that gets written during build time.
	Version = "dev"
	// CommitHash of the code that get written during build time.
	CommitHash     = ""
	oneOffFlag     = flag.Bool("oneoff", false, "Run the probe measurements one time and exit.")
	configPathFlag = flag.String("config-path", "/etc/astrolavos", "Specify the path of the config file.")
)

func main() {
	flag.Parse()

	// Initialize logging early for better error visibility
	initLogging("INFO") // Default level before config is loaded

	log.WithFields(log.Fields{
		"version": Version,
		"commit":  CommitHash,
	}).Info("Starting Astrolavos")

	cfg, err := config.NewConfig(*configPathFlag)
	if err != nil {
		log.WithError(err).Fatal("Failed to load configuration")
	}

	// Re-initialize logging with config level
	initLogging(cfg.LogLevel)

	a := machinery.NewAstrolavos(cfg.AppPort, cfg.Endpoints, cfg.PromPushGateway, *oneOffFlag)
	if err := a.Start(); err != nil {
		log.WithError(err).Fatal("Failed to start Astrolavos")
	}

	log.Info("Shutting down Astrolavos...")
}

// initLogging initializes our logging behavior with structured formatting.
func initLogging(logLevel string) {
	var l log.Level

	switch logLevel {
	case "DEBUG":
		l = log.DebugLevel
	case "WARNING", "WARN":
		l = log.WarnLevel
	case "INFO":
		l = log.InfoLevel
	case "ERROR":
		l = log.ErrorLevel
	default:
		l = log.InfoLevel
	}

	log.SetLevel(l)
	log.SetOutput(os.Stdout)
	log.SetFormatter(&log.JSONFormatter{
		TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		FieldMap: log.FieldMap{
			log.FieldKeyTime:  "timestamp",
			log.FieldKeyLevel: "level",
			log.FieldKeyMsg:   "message",
		},
	})
}

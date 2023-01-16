package main

import (
	"astrolavos/internal/config"
	"astrolavos/internal/machinery"
	"flag"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
)

var (
	// Version of the tool that gets written during build time
	Version = "dev"
	// CommitHash of the code that get written during build time
	CommitHash     = ""
	oneOffFlag     = flag.Bool("oneoff", false, "Run the probe measurements one time and exit.")
	configPathFlag = flag.String("config-path", "/etc/astrolavos", "Specify the path of the config file.")
)

func main() {

	flag.Parse()
	fmt.Printf("Starting Astrolavos version:%s - commit hash:%s\n", Version, CommitHash)

	cfg, err := config.NewConfig(*configPathFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error:%v\n", err)
		os.Exit(1)
	}

	initLogging(cfg.LogLevel)

	a := machinery.NewAstrolavos(cfg.AppPort, cfg.Endpoints, cfg.PromPushGateway, *oneOffFlag)
	if err := a.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error:%v\n", err)
		os.Exit(1)
	}
	log.Info("Shutting down Astrolavos...")

}

// initLogging initiliazes our logging behaviour
func initLogging(logLevel string) {

	var l log.Level
	switch logLevel {
	case "DEBUG":
		l = log.DebugLevel
	case "WARNING":
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
	log.WithFields(log.Fields{})
}

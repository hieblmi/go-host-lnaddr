package main

import (
	"sync"

	btclogv1 "github.com/btcsuite/btclog"
	btclogv2 "github.com/btcsuite/btclog/v2"
	"github.com/lightningnetwork/lnd/build"
)

var (
	initBackend sync.Once
	subLogMgr   *build.SubLoggerManager
	initError   error
)

/*
GetLogger ensure log backend is initialized and return a logger.
*/
func GetLogger(workingDir string, logger string) (btclogv1.Logger, error) {
	initLog(workingDir)
	if initError != nil {
		return nil, initError
	}

	gen := func(tag string) btclogv2.Logger {
		if subLogMgr == nil {
			return btclogv2.Disabled
		}
		return subLogMgr.GenSubLogger(tag, func() {})
	}
	return build.NewSubLogger(logger, gen), nil
}

func initLog(workingDir string) {
	initBackend.Do(func() {
		buildLogWriter := build.NewRotatingLogWriter()

		filename := workingDir + "/logs/lnaddr.log"
		cfg := &build.FileLoggerConfig{
			MaxLogFiles:    3,
			MaxLogFileSize: 10,
			Compressor:     "gzip",
		}
		err := buildLogWriter.InitLogRotator(cfg, filename)
		if err != nil {
			initError = err
			return
		}

		// Set up the root sub-logger manager using default handlers
		// that write to both the console and the rotating file.
		logCfg := build.DefaultLogConfig()
		handlers := build.NewDefaultLogHandlers(logCfg, buildLogWriter)
		subLogMgr = build.NewSubLoggerManager(handlers...)
	})
}

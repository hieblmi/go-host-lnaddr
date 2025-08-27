package main

import (
	"io"
	"sync"

	"github.com/btcsuite/btclog"
	"github.com/lightningnetwork/lnd/build"
)

var (
	initBackend sync.Once
	logWriter   *build.RotatingLogWriter
	initError   error
)

/*
Writer is the implementatino of io.Writer interface required for logging
*/
type Writer struct {
	writer io.Writer
}

func (w *Writer) Write(b []byte) (int, error) {
	if w.writer != nil {
		_, err := w.writer.Write(b)
		if err != nil {
			return 0, err
		}
	}
	return len(b), nil
}

/*
GetLogger ensure log backend is initialized and return a logger.
*/
func GetLogger(workingDir string, logger string) (btclog.Logger, error) {
	initLog(workingDir)
	if initError != nil {
		return nil, initError
	}
	return logWriter.GenSubLogger(logger, func() {}), nil
}

/*
GetLogWriter ensure log backend is initialized and return the writer singleton.
This writer is sent to other systems to they can use the same log file.
*/
func GetLogWriter(workingDir string) (*build.RotatingLogWriter, error) {
	initLog(workingDir)
	return logWriter, initError
}

func initLog(workingDir string) {
	initBackend.Do(func() {
		buildLogWriter := build.NewRotatingLogWriter()

		filename := workingDir + "/logs/lnaddr.log"
		err := buildLogWriter.InitLogRotator(filename, 10, 3)
		if err != nil {
			initError = err
			return
		}
		logWriter = buildLogWriter
	})
}

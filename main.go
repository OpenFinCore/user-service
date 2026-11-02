package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"ofc-user-service/conf"
	"ofc-user-service/logger"
	"ofc-user-service/router"
	"ofc-user-service/service"
)

func main() {
	fmt.Println("#==================================#")
	fmt.Println("#====== user-service ==========#")
	fmt.Println("#==================================#")

	container := service.NewServiceContainer()
	config := conf.GetConfig()

	fmt.Println("========== Database ready, tables migrated & seeded ==========")

	server := &http.Server{
		Addr:              config.HTTP.Addr,
		Handler:           router.New(container),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       time.Duration(config.HTTP.ReadTimeoutSeconds) * time.Second,
		WriteTimeout:      time.Duration(config.HTTP.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.LogInfo(fmt.Sprintf("HTTP server listening on %s (stage: %s)", config.HTTP.Addr, config.Stage), "")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.LogFatal("", err)
		}
	}()

	<-ctx.Done()
	logger.LogInfo("shutting down", "")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(config.HTTP.ShutdownTimeoutSeconds)*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.LogError("", fmt.Errorf("graceful shutdown failed: %w", err))
	}
}

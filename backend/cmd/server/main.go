package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"stravaapp/backend/internal/config"
	"stravaapp/backend/internal/server"
	"stravaapp/backend/internal/store"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		log.Fatalf("aws config error: %v", err)
	}

	dynamoClient := dynamodb.NewFromConfig(awsCfg)
	dynamoStore := store.New(
		dynamoClient,
		cfg.DynamoAthletesTable,
		cfg.DynamoActivitiesTable,
		cfg.DynamoActivitiesIndex,
		cfg.DynamoRoutesTable,
	)
	app := server.New(cfg, dynamoStore)
	httpServer := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      app.Handler(),
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 45 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("server listening on :%s", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
		os.Exit(1)
	}
}

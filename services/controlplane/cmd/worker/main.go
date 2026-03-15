package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/discordwell/evo-control-plane/services/controlplane/internal/catalog"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/config"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/domain"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/repo"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/service"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/storage"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/worker"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()

	cat, err := catalog.Load(cfg.CatalogPath)
	if err != nil {
		log.Fatalf("load catalog: %v", err)
	}

	postgres, err := repo.NewPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer postgres.Close()

	var objectStore storage.Store
	switch cfg.ObjectStore {
	case "s3":
		objectStore, err = storage.NewS3(ctx, cfg.S3Endpoint, cfg.S3Region, cfg.S3Bucket, cfg.S3AccessKeyID, cfg.S3SecretAccessKey)
	default:
		objectStore, err = storage.NewFS(cfg.ArtifactRoot)
	}
	if err != nil {
		log.Fatalf("object store: %v", err)
	}

	svc := service.New(postgres, objectStore, cat, domain.Org{
		ID:        cfg.DefaultOrgID,
		Name:      cfg.DefaultOrgName,
		Slug:      "default-org",
		CreatedAt: time.Now().UTC(),
	})
	svc.ConfigureBootstrapAdmin(cfg.DefaultAdminEmail, cfg.DefaultAdminPassword, cfg.DefaultAdminName)
	if err := svc.Bootstrap(ctx); err != nil {
		log.Fatalf("bootstrap: %v", err)
	}

	logger := log.New(os.Stdout, "", log.LstdFlags)
	if err := worker.New(svc, cfg.WorkerPoll, logger).Run(ctx); err != nil {
		log.Fatal(err)
	}
}

package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nano-backend/internal/config"
	"nano-backend/internal/database"
	"nano-backend/internal/handlers"
	"nano-backend/internal/jobs"
	"nano-backend/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("[config] No .env file found, using environment variables")
	}

	// Initialize config
	cfg := config.Load()

	// Initialize database
	if err := database.Init(cfg); err != nil {
		log.Fatalf("[database] Failed to initialize: %v", err)
	}
	defer database.Close()

	// Ensure initial admin
	if err := database.EnsureInitialAdmin(cfg); err != nil {
		log.Fatalf("[auth] Failed to create initial admin: %v", err)
	}

	// Create Fiber app
	app := fiber.New(fiber.Config{
		BodyLimit: 25 * 1024 * 1024, // 25MB
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			log.Printf("[error] %s %s - %v", c.Method(), c.Path(), err)
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		},
	})

	// Logger middleware with detailed request logging
	app.Use(logger.New(logger.Config{
		Format:     "[${time}] ${status} ${method} ${path} - ${latency} - ${ip} - ${ua}\n",
		TimeFormat: "2006-01-02 15:04:05",
		TimeZone:   "Local",
	}))

	// CORS
	app.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CorsOrigins,
		AllowCredentials: true,
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
		AllowMethods:     "GET, POST, PUT, PATCH, DELETE, OPTIONS",
	}))

	// Setup routes
	setupRoutes(app, cfg)

	// Start job runner
	jobs.StartJobRunner(cfg)

	// Start cleanup loops
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		// Run immediately
		database.CleanupExpiredSessions()
		database.CleanupExpiredFiles(cfg)
		for range ticker.C {
			database.CleanupExpiredSessions()
			database.CleanupExpiredFiles(cfg)
		}
	}()

	// Graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Println("[server] Shutting down...")
		app.Shutdown()
	}()

	// Start server
	log.Printf("[server] Starting on port %s", cfg.Port)
	log.Printf("[server] PUBLIC_BASE_URL = %s", cfg.PublicBaseURL)
	if err := app.Listen(":" + cfg.Port); err != nil {
		log.Fatalf("[server] Failed to start: %v", err)
	}
}

func setupRoutes(app *fiber.App, cfg *config.Config) {
	// Health check
	app.Get("/api/health", handlers.HealthCheck)

	// Auth routes (no auth required)
	app.Post("/api/auth/login", handlers.Login)

	// Auth middleware for protected routes
	authMiddleware := middleware.AuthMiddleware

	// Auth routes (auth required)
	app.Post("/api/auth/logout", authMiddleware, handlers.Logout)
	app.Get("/api/auth/me", authMiddleware, handlers.GetCurrentUser)

	// Models
	app.Get("/api/models", authMiddleware, handlers.GetModels)

	// Provider settings
	app.Get("/api/settings/provider", authMiddleware, handlers.GetProviderSettings)
	app.Put("/api/settings/provider", authMiddleware, handlers.UpdateProviderSettings)

	// Admin routes
	adminMiddleware := middleware.RequireAdmin
	app.Get("/api/admin/users", authMiddleware, adminMiddleware, handlers.AdminListUsers)
	app.Post("/api/admin/users", authMiddleware, adminMiddleware, handlers.AdminCreateUser)
	app.Delete("/api/admin/users/:id", authMiddleware, adminMiddleware, handlers.AdminDeleteUser)
	app.Patch("/api/admin/users/:id/status", authMiddleware, adminMiddleware, handlers.AdminUpdateUserStatus)
	app.Get("/api/admin/settings", authMiddleware, adminMiddleware, handlers.AdminGetSettings)
	app.Put("/api/admin/settings", authMiddleware, adminMiddleware, handlers.AdminUpdateSettings)

	// Generations
	app.Get("/api/generations", authMiddleware, handlers.ListGenerations)
	app.Get("/api/generations/:id", authMiddleware, handlers.GetGeneration)
	app.Patch("/api/generations/:id/favorite", authMiddleware, handlers.ToggleFavorite)
	app.Delete("/api/generations/:id", authMiddleware, handlers.DeleteGeneration)

	// Generate
	app.Post("/api/generate/image", authMiddleware, handlers.GenerateImage)
	app.Post("/api/generate/video", authMiddleware, handlers.GenerateVideo)

	// Video runs
	app.Get("/api/video/runs", authMiddleware, handlers.ListVideoRuns)
	app.Post("/api/video/runs", authMiddleware, handlers.CreateVideoRun)

	// Presets
	app.Get("/api/presets", authMiddleware, handlers.ListPresets)
	app.Post("/api/presets", authMiddleware, handlers.CreatePreset)
	app.Delete("/api/presets/:id", authMiddleware, handlers.DeletePreset)

	// Library
	app.Get("/api/library", authMiddleware, handlers.ListLibrary)
	app.Post("/api/library", authMiddleware, handlers.CreateLibraryItem)
	app.Delete("/api/library/:id", authMiddleware, handlers.DeleteLibraryItem)

	// Reference uploads
	app.Get("/api/reference-uploads", authMiddleware, handlers.ListReferenceUploads)
	app.Post("/api/reference-uploads", authMiddleware, handlers.CreateReferenceUploads)
	app.Delete("/api/reference-uploads/:id", authMiddleware, handlers.DeleteReferenceUpload)

	// Files (authenticated)
	app.Get("/api/files/:id", authMiddleware, handlers.GetFile)

	// Files (public - for provider to fetch reference images)
	app.Get("/public/files/:id", handlers.GetPublicFile)
}

package middleware

import (
	"log"
	"strings"

	"nano-backend/internal/database"
	"nano-backend/internal/models"

	"github.com/gofiber/fiber/v2"
)

// AuthMiddleware validates the user's token
func AuthMiddleware(c *fiber.Ctx) error {
	token := getTokenFromRequest(c)
	if token == "" {
		log.Printf("[auth] No token provided for %s %s", c.Method(), c.Path())
		return c.Status(401).JSON(fiber.Map{"error": "未登录或登录已过期"})
	}

	session, err := database.GetSession(token)
	if err != nil {
		log.Printf("[auth] Error getting session: %v", err)
		return c.Status(401).JSON(fiber.Map{"error": "未登录或登录已过期"})
	}
	if session == nil {
		log.Printf("[auth] Session not found for token")
		return c.Status(401).JSON(fiber.Map{"error": "未登录或登录已过期"})
	}

	// Check if session is expired
	if session.ExpiresAt < models.Now() {
		log.Printf("[auth] Session expired for user %s", session.UserID)
		return c.Status(401).JSON(fiber.Map{"error": "未登录或登录已过期"})
	}

	user, err := database.GetUserByID(session.UserID)
	if err != nil || user == nil {
		log.Printf("[auth] User not found: %s", session.UserID)
		return c.Status(401).JSON(fiber.Map{"error": "未登录或登录已过期"})
	}

	// Set user in context
	c.Locals("user", &models.SanitizedUser{
		ID:       user.ID,
		Username: user.Username,
		Role:     user.Role,
	})
	c.Locals("token", token)

	return c.Next()
}

// RequireAdmin checks if the user is an admin
func RequireAdmin(c *fiber.Ctx) error {
	user := c.Locals("user").(*models.SanitizedUser)
	if user.Role != "admin" {
		log.Printf("[auth] User %s attempted admin action without permission", user.Username)
		return c.Status(403).JSON(fiber.Map{"error": "无权限"})
	}
	return c.Next()
}

// GetCurrentUser returns the current user from context
func GetCurrentUser(c *fiber.Ctx) *models.SanitizedUser {
	user := c.Locals("user")
	if user == nil {
		return nil
	}
	return user.(*models.SanitizedUser)
}

// GetToken returns the current token from context
func GetToken(c *fiber.Ctx) string {
	token := c.Locals("token")
	if token == nil {
		return ""
	}
	return token.(string)
}

func getTokenFromRequest(c *fiber.Ctx) string {
	// Try Authorization header
	auth := c.Get("Authorization")
	if auth != "" {
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			return strings.TrimSpace(auth[7:])
		}
	}

	// Try query parameter
	token := c.Query("token")
	if token != "" {
		return token
	}

	return ""
}

package container

import (
	"database/sql"
	"fmt"

	"github.com/hveda/setagaya/setagaya/auth"
	"github.com/hveda/setagaya/setagaya/config"
	"github.com/hveda/setagaya/setagaya/repository"
	"github.com/hveda/setagaya/setagaya/service"
)

// Container manages application dependencies
type Container struct {
	// Database
	DB *sql.DB
	
	// Repositories
	ProjectRepo service.ProjectRepository
	
	// Services
	ProjectService service.ProjectService
	
	// Auth components
	SessionManager *auth.SecureSessionManager
}

// NewContainer creates and configures a new dependency injection container
func NewContainer() (*Container, error) {
	container := &Container{}
	
	// Initialize database connection
	if err := container.initDatabase(); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	
	// Initialize repositories
	container.initRepositories()
	
	// Initialize services
	container.initServices()
	
	// Initialize auth components
	if err := container.initAuthComponents(); err != nil {
		return nil, fmt.Errorf("failed to initialize auth components: %w", err)
	}
	
	return container, nil
}

// initDatabase initializes the database connection
func (c *Container) initDatabase() error {
	// This would use the existing database initialization from config
	// For now, we'll use a placeholder
	
	dbConfig := config.SC.DBConf
	if dbConfig == nil {
		return fmt.Errorf("database configuration not found")
	}
	
	// Connect to database (using existing connection logic)
	var err error
	c.DB, err = sql.Open("mysql", config.SC.DBEndpoint)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}
	
	// Test connection
	if err := c.DB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}
	
	return nil
}

// initRepositories initializes all repository instances
func (c *Container) initRepositories() {
	c.ProjectRepo = repository.NewProjectRepository(c.DB)
}

// initServices initializes all service instances
func (c *Container) initServices() {
	c.ProjectService = service.NewProjectService(c.ProjectRepo)
}

// initAuthComponents initializes authentication and security components
func (c *Container) initAuthComponents() error {
	// Initialize JWT configuration
	if err := auth.InitJWTConfig(); err != nil {
		return fmt.Errorf("failed to initialize JWT config: %w", err)
	}
	
	// Initialize secure session manager
	if err := auth.InitSecureSessionManager(); err != nil {
		return fmt.Errorf("failed to initialize session manager: %w", err)
	}
	c.SessionManager = auth.DefaultSessionManager
	
	return nil
}

// Close closes all resources managed by the container
func (c *Container) Close() error {
	var lastErr error
	
	// Close database connection
	if c.DB != nil {
		if err := c.DB.Close(); err != nil {
			lastErr = err
		}
	}
	
	return lastErr
}

// Health checks for monitoring
func (c *Container) HealthCheck() map[string]string {
	health := make(map[string]string)
	
	// Check database
	if c.DB != nil {
		if err := c.DB.Ping(); err != nil {
			health["database"] = "unhealthy: " + err.Error()
		} else {
			health["database"] = "healthy"
		}
	} else {
		health["database"] = "not initialized"
	}
	
	// Check other components
	if c.ProjectService != nil {
		health["project_service"] = "healthy"
	} else {
		health["project_service"] = "not initialized"
	}
	
	if c.SessionManager != nil {
		health["session_manager"] = "healthy"
	} else {
		health["session_manager"] = "not initialized"
	}
	
	return health
}

// ServiceContainer provides a global container instance
var ServiceContainer *Container

// InitServiceContainer initializes the global service container
func InitServiceContainer() error {
	var err error
	ServiceContainer, err = NewContainer()
	if err != nil {
		return fmt.Errorf("failed to create service container: %w", err)
	}
	return nil
}

// GetServiceContainer returns the global service container
func GetServiceContainer() *Container {
	return ServiceContainer
}

// CloseServiceContainer closes the global service container
func CloseServiceContainer() error {
	if ServiceContainer != nil {
		return ServiceContainer.Close()
	}
	return nil
}
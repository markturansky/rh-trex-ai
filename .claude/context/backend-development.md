# TRex Backend Development Standards

## Framework Architecture

### Generated Code Structure
TRex follows a strict layered architecture with automatic code generation:

```go
// Layer 1: Handlers (HTTP/gRPC endpoints)
// File: pkg/handlers/{entity}.go
func (h EntityHandler) Create(w http.ResponseWriter, r *http.Request) {
    entity, serviceErr := h.service.Create(r.Context(), request)
    if serviceErr != nil {
        errors.GeneralError(r, w, serviceErr.Code, serviceErr.Error())
        return
    }
    presenters.PresentEntity(w, entity)
}

// Layer 2: Services (Business logic)
// File: pkg/services/{entity}.go  
func (s *sqlEntityService) Create(ctx context.Context, entity *api.Entity) (*api.Entity, *errors.ServiceError) {
    // Validate business rules
    if err := s.validateEntity(entity); err != nil {
        return nil, errors.BadRequest(err.Error())
    }
    
    // Persist via DAO
    return s.dao.Create(ctx, entity)
}

// Layer 3: DAO (Data Access Objects)
// File: pkg/dao/{entity}.go
func (d *sqlEntityDao) Create(ctx context.Context, entity *api.Entity) (*api.Entity, *errors.ServiceError) {
    dbEntity := d.mapToDBModel(entity)
    
    if err := d.g2.WithContext(ctx).Create(dbEntity).Error; err != nil {
        return nil, errors.GeneralError("Failed to create entity: %w", err)
    }
    
    return d.mapFromDBModel(dbEntity), nil
}

// Layer 4: Models (Database entities)
// File: pkg/api/{entity}.go
type Entity struct {
    Meta
    Name        string    `json:"name" gorm:"index"`
    Description string    `json:"description"`
    CreatedBy   string    `json:"created_by"`
}
```

### Plugin-Based Auto-Registration
All entities use the plugin system for auto-discovery:

```go
// File: plugins/{entities}/plugin.go
package entities

import (
    "github.com/openshift-online/rh-trex-ai/cmd/trex/environments"
    "github.com/openshift-online/rh-trex-ai/pkg/handlers"
)

func init() {
    // Auto-register routes
    environments.RegisterRoutes(func(s *environments.APIServerConfig) {
        entityHandler := handlers.NewEntityHandler(s.Services.Entities())
        
        s.Router.HandleFunc("/api/rh-trex/v1/entities", entityHandler.List).Methods("GET")
        s.Router.HandleFunc("/api/rh-trex/v1/entities", entityHandler.Create).Methods("POST")
        s.Router.HandleFunc("/api/rh-trex/v1/entities/{id}", entityHandler.Get).Methods("GET")
        s.Router.HandleFunc("/api/rh-trex/v1/entities/{id}", entityHandler.Update).Methods("PATCH")
        s.Router.HandleFunc("/api/rh-trex/v1/entities/{id}", entityHandler.Delete).Methods("DELETE")
    })
    
    // Auto-register controllers for event handling
    environments.RegisterControllers(func(s *environments.ControllerServerConfig) {
        entityService := s.Services.Entities()
        s.ControllerManager.Add(&controllers.ControllerConfig{
            Source: "Entities",
            Handlers: map[api.EventType][]controllers.ControllerHandlerFunc{
                api.CreateEventType: {entityService.OnUpsert},
                api.UpdateEventType: {entityService.OnUpsert},
                api.DeleteEventType: {entityService.OnDelete},
            },
        })
    })
    
    // Auto-register presenters
    environments.RegisterPresenters(func(r chi.Router) {
        presenters.RegisterEntityPresenters(r)
    })
}
```

## Error Handling Patterns

### TRex Error Types
Use standardized error types from `pkg/errors`:

```go
// Bad Request (400) - Client input validation
if entity.Name == "" {
    return nil, errors.BadRequest("Entity name is required")
}

// Unauthorized (401) - Authentication failure  
if !isValidToken(token) {
    return nil, errors.Unauthenticated("Invalid authentication token")
}

// Forbidden (403) - Authorization failure
if !hasPermission(user, "entities:create") {
    return nil, errors.Forbidden("Insufficient permissions to create entity")
}

// Not Found (404) - Resource doesn't exist
if entity == nil {
    return nil, errors.NotFound("Entity not found: %s", id)
}

// Conflict (409) - Resource already exists or constraint violation
if isDuplicate(err) {
    return nil, errors.Conflict("Entity with name '%s' already exists", entity.Name)
}

// Unprocessable Entity (422) - Business logic violation
if !isValidBusinessRule(entity) {
    return nil, errors.UnprocessableEntity("Entity violates business rule: %s", rule)
}

// Internal Server Error (500) - Unexpected errors
if err != nil {
    return nil, errors.GeneralError("Failed to process entity: %w", err)
}
```

### Error Response Format
All errors follow OpenAPI standard format:

```go
type ServiceError struct {
    Code        string `json:"code"`
    Reason      string `json:"reason"`
    OperationID string `json:"operation_id,omitempty"`
}
```

## Database Transaction Patterns

### Single Resource Operations
```go
func (s *sqlEntityService) Create(ctx context.Context, entity *api.Entity) (*api.Entity, *errors.ServiceError) {
    // Simple operations use auto-transactions via GORM
    return s.dao.Create(ctx, entity)
}
```

### Multi-Resource Operations
```go
func (s *sqlEntityService) CreateWithRelated(ctx context.Context, req *CreateRequest) (*api.Entity, *errors.ServiceError) {
    var result *api.Entity
    
    err := s.dao.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        // Create main entity
        entity := &api.Entity{Name: req.Name}
        if err := tx.Create(entity).Error; err != nil {
            return err
        }
        
        // Create related resources
        for _, relatedData := range req.Related {
            related := &api.Related{
                EntityID: entity.ID,
                Data:     relatedData,
            }
            if err := tx.Create(related).Error; err != nil {
                return err
            }
        }
        
        result = entity
        return nil
    })
    
    if err != nil {
        return nil, errors.GeneralError("Transaction failed: %w", err)
    }
    
    return result, nil
}
```

### Advisory Locks for Concurrency
```go
func (s *sqlEntityService) ProcessWithLock(ctx context.Context, entityID string) error {
    lockKey := fmt.Sprintf("entity_process_%s", entityID)
    
    return s.dao.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        // Acquire advisory lock
        var acquired bool
        if err := tx.Raw("SELECT pg_try_advisory_xact_lock(?)", lockKey).Scan(&acquired).Error; err != nil {
            return err
        }
        if !acquired {
            return errors.Conflict("Entity is being processed by another request")
        }
        
        // Perform locked operation
        return s.performLockedOperation(ctx, tx, entityID)
    })
}
```

## gRPC Service Patterns

### Unary RPC Implementation
```go
// File: pkg/server/grpc_server.go
func (s *GRPCServer) CreateEntity(ctx context.Context, req *rhtrexv1.CreateEntityRequest) (*rhtrexv1.Entity, error) {
    // Convert gRPC request to internal model
    entity := &api.Entity{
        Name:        req.Name,
        Description: req.Description,
    }
    
    // Call service layer
    created, serviceErr := s.entityService.Create(ctx, entity)
    if serviceErr != nil {
        return nil, status.Error(grpcCodeFromServiceError(serviceErr.Code), serviceErr.Reason)
    }
    
    // Convert back to gRPC response
    return &rhtrexv1.Entity{
        Id:          created.ID,
        Name:        created.Name,
        Description: created.Description,
        CreatedAt:   timestamppb.New(created.CreatedAt),
    }, nil
}
```

### Server Streaming RPC
```go
func (s *GRPCServer) WatchEntities(req *rhtrexv1.WatchEntitiesRequest, stream rhtrexv1.EntityService_WatchEntitiesServer) error {
    // Subscribe to entity events
    ch := make(chan *api.Entity, 10)
    unsubscribe := s.eventBroker.Subscribe("entities", ch)
    defer unsubscribe()
    
    for {
        select {
        case <-stream.Context().Done():
            return stream.Context().Err()
        case entity := <-ch:
            grpcEntity := &rhtrexv1.Entity{
                Id:   entity.ID,
                Name: entity.Name,
            }
            if err := stream.Send(grpcEntity); err != nil {
                return err
            }
        }
    }
}
```

## Event-Driven Architecture

### Idempotent Event Handlers
```go
// File: pkg/services/{entity}.go
func (s *sqlEntityService) OnUpsert(ctx context.Context, id string) error {
    logger := logger.NewOCMLogger(ctx)
    
    entity, err := s.dao.Get(ctx, id)
    if err != nil {
        if errors.IsNotFound(err) {
            // Entity deleted between event and processing
            logger.Infof("Entity %s no longer exists, skipping upsert handler", id)
            return nil
        }
        return err
    }
    
    // Idempotent business logic - safe to run multiple times
    logger.Infof("Processing entity upsert: %s (name: %s)", entity.ID, entity.Name)
    
    // Example: Update external system
    if err := s.externalClient.SyncEntity(ctx, entity); err != nil {
        // Log but don't fail - will retry on next event
        logger.Errorf("Failed to sync entity %s to external system: %v", entity.ID, err)
        return err
    }
    
    return nil
}

func (s *sqlEntityService) OnDelete(ctx context.Context, id string) error {
    logger := logger.NewOCMLogger(ctx)
    logger.Infof("Processing entity deletion: %s", id)
    
    // Cleanup external resources - idempotent
    if err := s.externalClient.DeleteEntity(ctx, id); err != nil {
        // Check if already deleted (idempotent)
        if !isNotFoundError(err) {
            return err
        }
    }
    
    return nil
}
```

### Event Broker Usage
```go
// File: pkg/server/event_broker.go  
type EventBroker struct {
    subscribers map[string][]chan *api.Entity
    mu          sync.RWMutex
}

func (b *EventBroker) Subscribe(topic string, ch chan *api.Entity) func() {
    b.mu.Lock()
    defer b.mu.Unlock()
    
    b.subscribers[topic] = append(b.subscribers[topic], ch)
    
    // Return unsubscribe function
    return func() {
        b.mu.Lock()
        defer b.mu.Unlock()
        
        subs := b.subscribers[topic]
        for i, sub := range subs {
            if sub == ch {
                b.subscribers[topic] = append(subs[:i], subs[i+1:]...)
                close(ch)
                break
            }
        }
    }
}
```

## Service Locator Pattern

### Dependency Injection
```go
// File: cmd/trex/environments/environment.go
type ServiceRegistry struct {
    entityService api.EntityService
}

func (r *ServiceRegistry) Entities() api.EntityService {
    if r.entityService == nil {
        entityDao := dao.NewEntityDao(&dao.EntityDaoConfig{
            DB: r.Database.New(),
        })
        
        r.entityService = services.NewEntityService(services.EntityServiceConfig{
            EntityDao: entityDao,
        })
    }
    return r.entityService
}
```

## Best Practices

### Context Usage
```go
// Always pass context through all layers
func (s *service) ProcessEntity(ctx context.Context, id string) error {
    // Check for cancellation
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }
    
    // Pass context to all downstream calls
    entity, err := s.dao.Get(ctx, id)
    if err != nil {
        return err
    }
    
    return s.externalService.Update(ctx, entity)
}
```

### Resource Cleanup
```go
func (s *service) ProcessLargeDataset(ctx context.Context) error {
    file, err := os.Open("large-dataset.csv")
    if err != nil {
        return err
    }
    defer file.Close() // Always defer cleanup
    
    reader := csv.NewReader(file)
    defer func() {
        // Additional cleanup if needed
        if err := reader.Close(); err != nil {
            log.Printf("Error closing reader: %v", err)
        }
    }()
    
    // Process data...
    return nil
}
```

### Structured Logging
```go
import "github.com/openshift-online/rh-trex-ai/pkg/logger"

func (s *service) ProcessEntity(ctx context.Context, id string) error {
    logger := logger.NewOCMLogger(ctx)
    
    logger.Infof("Starting entity processing: %s", id)
    defer func() {
        logger.Infof("Completed entity processing: %s", id)
    }()
    
    entity, err := s.dao.Get(ctx, id)
    if err != nil {
        logger.Errorf("Failed to get entity %s: %v", id, err)
        return err
    }
    
    logger.V(10).Infof("Processing entity: %+v", entity)
    return nil
}
```
# TRex Database Development Standards

## Migration Patterns

### Reversible Migration Structure
Every migration must be reversible with proper Up() and Down() methods:

```go
// File: pkg/db/migrations/YYYYMMDDHHMM_add_entities.go
package migrations

import (
    "github.com/go-gormigrate/gormigrate/v2"
    "gorm.io/gorm"
    "github.com/openshift-online/rh-trex-ai/pkg/api"
)

func addEntities() *gormigrate.Migration {
    type Entity struct {
        api.Model
        Name        string `gorm:"index:idx_entity_name,unique"`
        Description string
        Status      string `gorm:"default:'active'"`
        Metadata    string `gorm:"type:jsonb"`
    }

    return &gormigrate.Migration{
        ID: "202501281200_add_entities",
        Migrate: func(tx *gorm.DB) error {
            return tx.AutoMigrate(&Entity{})
        },
        Rollback: func(tx *gorm.DB) error {
            return tx.Migrator().DropTable(&Entity{})
        },
    }
}
```

### Migration Registration
Migrations must be registered in migration_structs.go:

```go
// File: pkg/db/migrations/migration_structs.go
var MigrationList = []*gormigrate.Migration{
    // ... existing migrations
    addEntities(),
}
```

### Complex Migration Patterns
```go
// Column addition with data transformation
func addEntityTypeColumn() *gormigrate.Migration {
    return &gormigrate.Migration{
        ID: "202501281300_add_entity_type",
        Migrate: func(tx *gorm.DB) error {
            // Add column
            if err := tx.Exec("ALTER TABLE entities ADD COLUMN entity_type VARCHAR(50)").Error; err != nil {
                return err
            }
            
            // Set default values based on existing data
            if err := tx.Exec(`
                UPDATE entities 
                SET entity_type = CASE 
                    WHEN name LIKE 'user_%' THEN 'user'
                    WHEN name LIKE 'admin_%' THEN 'admin'
                    ELSE 'standard'
                END
            `).Error; err != nil {
                return err
            }
            
            // Add NOT NULL constraint
            return tx.Exec("ALTER TABLE entities ALTER COLUMN entity_type SET NOT NULL").Error
        },
        Rollback: func(tx *gorm.DB) error {
            return tx.Exec("ALTER TABLE entities DROP COLUMN entity_type").Error
        },
    }
}

// Index creation/deletion
func addEntityIndexes() *gormigrate.Migration {
    return &gormigrate.Migration{
        ID: "202501281400_add_entity_indexes",
        Migrate: func(tx *gorm.DB) error {
            // Composite index for common queries
            if err := tx.Exec("CREATE INDEX idx_entities_status_created_at ON entities(status, created_at)").Error; err != nil {
                return err
            }
            
            // Partial index for active entities only
            return tx.Exec("CREATE INDEX idx_entities_active_name ON entities(name) WHERE status = 'active'").Error
        },
        Rollback: func(tx *gorm.DB) error {
            if err := tx.Exec("DROP INDEX IF EXISTS idx_entities_active_name").Error; err != nil {
                return err
            }
            return tx.Exec("DROP INDEX IF EXISTS idx_entities_status_created_at").Error
        },
    }
}
```

## GORM Model Patterns

### Base Model Structure
```go
// File: pkg/api/entity.go
type Entity struct {
    Meta                                    // Includes ID, CreatedAt, UpdatedAt
    Name        string    `json:"name" gorm:"index:idx_entity_name,unique;not null;size:255"`
    Description string    `json:"description" gorm:"size:1000"`
    Status      string    `json:"status" gorm:"default:'active';size:50"`
    Metadata    string    `json:"metadata" gorm:"type:jsonb"`
    CreatedBy   string    `json:"created_by" gorm:"size:255;index"`
    
    // Foreign keys
    CategoryID  string    `json:"category_id" gorm:"type:uuid;index"`
    Category    *Category `json:"category,omitempty" gorm:"foreignKey:CategoryID"`
    
    // One-to-many relationships
    Items       []Item    `json:"items,omitempty" gorm:"foreignKey:EntityID"`
}

// Table name override
func (Entity) TableName() string {
    return "entities"
}

// GORM hooks for lifecycle management
func (e *Entity) BeforeCreate(tx *gorm.DB) error {
    if e.ID == "" {
        e.ID = uuid.New().String()
    }
    if e.Status == "" {
        e.Status = "active"
    }
    return nil
}

func (e *Entity) AfterCreate(tx *gorm.DB) error {
    // Trigger event notification
    return triggerEntityEvent(tx, "create", e.ID)
}
```

### Validation Tags and Constraints
```go
type Entity struct {
    Meta
    Name        string    `json:"name" gorm:"index:idx_entity_name,unique;not null;size:255" validate:"required,min=1,max=255"`
    Email       string    `json:"email" gorm:"index;size:320" validate:"email"`
    Status      string    `json:"status" gorm:"default:'active';size:50" validate:"oneof=active inactive pending"`
    Priority    int       `json:"priority" gorm:"default:0" validate:"min=0,max=10"`
    Config      string    `json:"config" gorm:"type:jsonb" validate:"json"`
    URL         string    `json:"url" gorm:"size:2048" validate:"url"`
    Tags        pq.StringArray `json:"tags" gorm:"type:text[]"`
}
```

### Relationship Patterns
```go
// One-to-Many
type Category struct {
    Meta
    Name     string   `json:"name"`
    Entities []Entity `json:"entities,omitempty" gorm:"foreignKey:CategoryID"`
}

// Many-to-Many with join table
type Tag struct {
    Meta
    Name     string   `json:"name"`
    Entities []Entity `json:"entities,omitempty" gorm:"many2many:entity_tags"`
}

// Polymorphic relationships
type Comment struct {
    Meta
    Content       string `json:"content"`
    CommentableID string `json:"commentable_id"`
    CommentableType string `json:"commentable_type"`
}

type Entity struct {
    Meta
    Name     string    `json:"name"`
    Comments []Comment `json:"comments,omitempty" gorm:"polymorphic:Commentable"`
}
```

## DAO Implementation Patterns

### Standard CRUD Operations
```go
// File: pkg/dao/entity.go
type sqlEntityDao struct {
    g2 func() *gorm.DB
}

func (d *sqlEntityDao) Create(ctx context.Context, entity *api.Entity) (*api.Entity, *errors.ServiceError) {
    if err := d.g2().WithContext(ctx).Create(entity).Error; err != nil {
        if IsUniqueConstraintError(err) {
            return nil, errors.Conflict("Entity with name '%s' already exists", entity.Name)
        }
        return nil, errors.GeneralError("Failed to create entity: %w", err)
    }
    
    return entity, nil
}

func (d *sqlEntityDao) Get(ctx context.Context, id string) (*api.Entity, *errors.ServiceError) {
    entity := &api.Entity{}
    
    if err := d.g2().WithContext(ctx).First(entity, "id = ?", id).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, errors.NotFound("Entity not found: %s", id)
        }
        return nil, errors.GeneralError("Failed to get entity: %w", err)
    }
    
    return entity, nil
}

func (d *sqlEntityDao) Update(ctx context.Context, entity *api.Entity) (*api.Entity, *errors.ServiceError) {
    result := d.g2().WithContext(ctx).Save(entity)
    if result.Error != nil {
        if IsUniqueConstraintError(result.Error) {
            return nil, errors.Conflict("Entity name already exists")
        }
        return nil, errors.GeneralError("Failed to update entity: %w", result.Error)
    }
    
    if result.RowsAffected == 0 {
        return nil, errors.NotFound("Entity not found: %s", entity.ID)
    }
    
    return entity, nil
}

func (d *sqlEntityDao) Delete(ctx context.Context, id string) *errors.ServiceError {
    result := d.g2().WithContext(ctx).Delete(&api.Entity{}, "id = ?", id)
    if result.Error != nil {
        return errors.GeneralError("Failed to delete entity: %w", result.Error)
    }
    
    if result.RowsAffected == 0 {
        return errors.NotFound("Entity not found: %s", id)
    }
    
    return nil
}
```

### Query Patterns with Pagination
```go
func (d *sqlEntityDao) List(ctx context.Context, options *api.ListOptions) (*api.EntityList, *errors.ServiceError) {
    var entities []api.Entity
    var total int64
    
    query := d.g2().WithContext(ctx).Model(&api.Entity{})
    
    // Apply filters
    if options.Name != "" {
        query = query.Where("name ILIKE ?", "%"+options.Name+"%")
    }
    if options.Status != "" {
        query = query.Where("status = ?", options.Status)
    }
    if options.CreatedBy != "" {
        query = query.Where("created_by = ?", options.CreatedBy)
    }
    
    // Get total count (before pagination)
    if err := query.Count(&total).Error; err != nil {
        return nil, errors.GeneralError("Failed to count entities: %w", err)
    }
    
    // Apply sorting
    orderBy := "created_at DESC"
    if options.OrderBy != "" {
        orderBy = options.OrderBy
        if options.Order == "asc" {
            orderBy += " ASC"
        } else {
            orderBy += " DESC"
        }
    }
    query = query.Order(orderBy)
    
    // Apply pagination
    offset := (options.Page - 1) * options.Size
    query = query.Offset(offset).Limit(options.Size)
    
    // Include related data
    if options.IncludeCategory {
        query = query.Preload("Category")
    }
    if options.IncludeItems {
        query = query.Preload("Items")
    }
    
    if err := query.Find(&entities).Error; err != nil {
        return nil, errors.GeneralError("Failed to list entities: %w", err)
    }
    
    return &api.EntityList{
        Kind:  "EntityList",
        Items: entities,
        Page:  options.Page,
        Size:  options.Size,
        Total: int(total),
    }, nil
}
```

### Complex Queries and Joins
```go
func (d *sqlEntityDao) GetEntitiesWithStats(ctx context.Context) ([]api.EntityWithStats, *errors.ServiceError) {
    var results []api.EntityWithStats
    
    query := `
        SELECT 
            e.id,
            e.name,
            e.status,
            e.created_at,
            COUNT(i.id) as item_count,
            COALESCE(AVG(r.rating), 0) as avg_rating
        FROM entities e
        LEFT JOIN items i ON e.id = i.entity_id
        LEFT JOIN ratings r ON e.id = r.entity_id
        WHERE e.status = ?
        GROUP BY e.id, e.name, e.status, e.created_at
        ORDER BY item_count DESC, avg_rating DESC
    `
    
    if err := d.g2().WithContext(ctx).Raw(query, "active").Scan(&results).Error; err != nil {
        return nil, errors.GeneralError("Failed to get entity stats: %w", err)
    }
    
    return results, nil
}
```

### Transaction Patterns
```go
func (d *sqlEntityDao) CreateEntityWithItems(ctx context.Context, entity *api.Entity, items []api.Item) (*api.Entity, *errors.ServiceError) {
    err := d.g2().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        // Create main entity
        if err := tx.Create(entity).Error; err != nil {
            return err
        }
        
        // Create related items
        for i := range items {
            items[i].EntityID = entity.ID
            if err := tx.Create(&items[i]).Error; err != nil {
                return err
            }
        }
        
        // Update entity stats
        return tx.Model(entity).Update("item_count", len(items)).Error
    })
    
    if err != nil {
        return nil, errors.GeneralError("Failed to create entity with items: %w", err)
    }
    
    return entity, nil
}
```

## Event Notification Patterns

### PostgreSQL LISTEN/NOTIFY Integration
```go
// File: pkg/dao/events.go
func triggerEntityEvent(tx *gorm.DB, eventType, entityID string) error {
    eventData := map[string]string{
        "type":   eventType,
        "id":     entityID,
        "source": "entities",
    }
    
    jsonData, err := json.Marshal(eventData)
    if err != nil {
        return err
    }
    
    return tx.Exec("SELECT pg_notify(?, ?)", "entity_events", string(jsonData)).Error
}

// GORM hooks for automatic event triggering
func (e *Entity) AfterCreate(tx *gorm.DB) error {
    return triggerEntityEvent(tx, "create", e.ID)
}

func (e *Entity) AfterUpdate(tx *gorm.DB) error {
    return triggerEntityEvent(tx, "update", e.ID)
}

func (e *Entity) AfterDelete(tx *gorm.DB) error {
    return triggerEntityEvent(tx, "delete", e.ID)
}
```

## Database Configuration

### Connection Pool Settings
```go
// File: pkg/config/database.go
type DatabaseConfig struct {
    Name               string `yaml:"name" env:"DB_NAME"`
    Host               string `yaml:"host" env:"DB_HOST"`
    Port               int    `yaml:"port" env:"DB_PORT"`
    User               string `yaml:"user" env:"DB_USER"`
    Password           string `yaml:"password" env:"DB_PASSWORD"`
    SSLMode            string `yaml:"sslmode" env:"DB_SSLMODE"`
    MaxOpenConnections int    `yaml:"max_open_connections" env:"DB_MAX_OPEN_CONNECTIONS"`
    MaxIdleConnections int    `yaml:"max_idle_connections" env:"DB_MAX_IDLE_CONNECTIONS"`
    ConnMaxLifetime    int    `yaml:"conn_max_lifetime" env:"DB_CONN_MAX_LIFETIME"`
    Debug              bool   `yaml:"debug" env:"DB_DEBUG"`
}
```

### Connection String with SSL
```go
func (c *DatabaseConfig) ConnectionString(useSSL bool) string {
    sslMode := "disable"
    if useSSL && c.SSLMode != "" {
        sslMode = c.SSLMode
    }
    
    return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
        c.Host, c.Port, c.User, c.Password, c.Name, sslMode)
}
```

## Performance Optimization

### Query Optimization
```go
// Bad: N+1 query problem
func (d *sqlEntityDao) GetEntitiesWithCategoryBad(ctx context.Context) ([]api.Entity, error) {
    var entities []api.Entity
    d.g2().WithContext(ctx).Find(&entities)
    
    // This will execute N queries for N entities
    for i := range entities {
        d.g2().WithContext(ctx).Where("id = ?", entities[i].CategoryID).First(&entities[i].Category)
    }
    
    return entities, nil
}

// Good: Use preloading
func (d *sqlEntityDao) GetEntitiesWithCategoryGood(ctx context.Context) ([]api.Entity, error) {
    var entities []api.Entity
    
    // Single query with JOIN
    err := d.g2().WithContext(ctx).
        Preload("Category").
        Find(&entities).Error
        
    return entities, err
}
```

### Index Usage Guidelines
```go
// Ensure indexes match common query patterns
type Entity struct {
    Meta
    Name      string `gorm:"index:idx_entity_name"`           // Single column index
    Status    string `gorm:"index:idx_entity_status_created"` // Part of composite index
    CreatedAt time.Time `gorm:"index:idx_entity_status_created"` // Composite index
    Active    bool   `gorm:"index:idx_entity_active" sql:"default:true"`
}

// For queries like:
// WHERE status = 'active' AND created_at > '2023-01-01'
// The composite index idx_entity_status_created will be used efficiently
```

### Bulk Operations
```go
func (d *sqlEntityDao) BulkCreate(ctx context.Context, entities []api.Entity) *errors.ServiceError {
    batchSize := 100
    
    for i := 0; i < len(entities); i += batchSize {
        end := i + batchSize
        if end > len(entities) {
            end = len(entities)
        }
        
        batch := entities[i:end]
        if err := d.g2().WithContext(ctx).CreateInBatches(batch, batchSize).Error; err != nil {
            return errors.GeneralError("Failed to bulk create entities: %w", err)
        }
    }
    
    return nil
}
```

## Testing Database Code

### Test Database Setup
```go
// File: test/integration/entity_test.go
func TestEntityDAO(t *testing.T) {
    // Use test database
    testDB := test.NewTestDB()
    defer testDB.Close()
    
    // Run migrations
    if err := testDB.Migrate(); err != nil {
        t.Fatalf("Failed to migrate test DB: %v", err)
    }
    
    // Create DAO with test DB
    dao := NewEntityDao(&EntityDaoConfig{DB: testDB.NewSession})
    
    // Test operations
    entity := &api.Entity{Name: "test-entity"}
    created, err := dao.Create(context.Background(), entity)
    assert.NoError(t, err)
    assert.NotEmpty(t, created.ID)
}
```

### Transaction Testing
```go
func TestEntityDAOTransaction(t *testing.T) {
    testDB := test.NewTestDB()
    defer testDB.Close()
    
    dao := NewEntityDao(&EntityDaoConfig{DB: testDB.NewSession})
    
    // Test rollback on error
    err := dao.g2().Transaction(func(tx *gorm.DB) error {
        entity := &api.Entity{Name: "test"}
        if err := tx.Create(entity).Error; err != nil {
            return err
        }
        
        // Simulate error that should rollback
        return fmt.Errorf("simulated error")
    })
    
    assert.Error(t, err)
    
    // Verify rollback - entity should not exist
    var count int64
    dao.g2().Model(&api.Entity{}).Count(&count)
    assert.Equal(t, int64(0), count)
}
```
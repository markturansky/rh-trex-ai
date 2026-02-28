# TRex Error Handling Patterns

## HTTP Status Code Standards

### Standard Response Codes
TRex follows RFC 7231 HTTP status codes with specific patterns for REST APIs:

```go
// 200 OK - Successful GET, PATCH (with content)
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
    entity, err := h.service.Get(r.Context(), id)
    if err != nil {
        errors.HandleServiceError(r, w, err)
        return
    }
    presenters.Present(w, entity) // Returns 200 with content
}

// 201 Created - Successful POST (resource creation)
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
    created, err := h.service.Create(r.Context(), request)
    if err != nil {
        errors.HandleServiceError(r, w, err)
        return
    }
    w.Header().Set("Location", fmt.Sprintf("/api/rh-trex/v1/entities/%s", created.ID))
    w.WriteHeader(http.StatusCreated)
    presenters.Present(w, created)
}

// 204 No Content - Successful DELETE, PATCH (no content)
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
    err := h.service.Delete(r.Context(), id)
    if err != nil {
        errors.HandleServiceError(r, w, err)
        return
    }
    w.WriteHeader(http.StatusNoContent) // No response body
}

// 400 Bad Request - Client input validation errors
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
    var request CreateRequest
    if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
        errors.BadRequest(r, w, "Invalid JSON format")
        return
    }
    
    if request.Name == "" {
        errors.BadRequest(r, w, "Name is required")
        return
    }
}

// 401 Unauthorized - Missing or invalid authentication
func AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := extractToken(r)
        if token == "" {
            errors.Unauthenticated(r, w, "Authorization header required")
            return
        }
        
        if !isValidToken(token) {
            errors.Unauthenticated(r, w, "Invalid authentication token")
            return
        }
        
        next.ServeHTTP(w, r)
    })
}

// 403 Forbidden - Valid authentication but insufficient permissions
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
    user := getUserFromContext(r.Context())
    entity, err := h.service.Get(r.Context(), id)
    if err != nil {
        errors.HandleServiceError(r, w, err)
        return
    }
    
    if entity.CreatedBy != user.ID && !user.HasRole("admin") {
        errors.Forbidden(r, w, "Cannot delete entities created by other users")
        return
    }
    
    // Proceed with deletion...
}

// 404 Not Found - Resource doesn't exist
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
    entity, err := h.service.Get(r.Context(), id)
    if err != nil {
        if errors.IsNotFoundError(err) {
            errors.NotFound(r, w, "Entity not found: %s", id)
            return
        }
        errors.HandleServiceError(r, w, err)
        return
    }
    
    presenters.Present(w, entity)
}

// 409 Conflict - Resource already exists or constraint violation
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
    created, err := h.service.Create(r.Context(), request)
    if err != nil {
        if errors.IsConflictError(err) {
            errors.Conflict(r, w, "Entity with name '%s' already exists", request.Name)
            return
        }
        errors.HandleServiceError(r, w, err)
        return
    }
    
    presenters.Present(w, created)
}

// 422 Unprocessable Entity - Valid JSON but business logic violation
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
    var request UpdateRequest
    if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
        errors.BadRequest(r, w, "Invalid JSON format")
        return
    }
    
    // Business logic validation
    if request.EndDate != nil && request.StartDate != nil {
        if request.EndDate.Before(*request.StartDate) {
            errors.UnprocessableEntity(r, w, "End date cannot be before start date")
            return
        }
    }
    
    updated, err := h.service.Update(r.Context(), request)
    if err != nil {
        errors.HandleServiceError(r, w, err)
        return
    }
    
    presenters.Present(w, updated)
}

// 429 Too Many Requests - Rate limit exceeded
func RateLimitMiddleware(next http.Handler) http.Handler {
    limiter := rate.NewLimiter(rate.Limit(100), 200)
    
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !limiter.Allow() {
            errors.TooManyRequests(r, w, "Rate limit exceeded. Try again later.")
            return
        }
        next.ServeHTTP(w, r)
    })
}

// 500 Internal Server Error - Unexpected server errors
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
    entities, err := h.service.List(r.Context(), options)
    if err != nil {
        // Log the actual error for debugging
        logger := logger.NewOCMLogger(r.Context())
        logger.Errorf("Failed to list entities: %v", err)
        
        // Return generic error message to client
        errors.GeneralError(r, w, "Failed to retrieve entities")
        return
    }
    
    presenters.Present(w, entities)
}
```

## TRex Error Types and Usage

### Service Error Structure
```go
// File: pkg/errors/service_error.go
type ServiceError struct {
    Code        string `json:"code"`
    Reason      string `json:"reason"`
    OperationID string `json:"operation_id,omitempty"`
    HTTPStatus  int    `json:"-"` // Not serialized in response
}

func (e *ServiceError) Error() string {
    return e.Reason
}

// Standard error constructors
func BadRequest(format string, args ...interface{}) *ServiceError {
    return &ServiceError{
        Code:       "TREX-400",
        Reason:     fmt.Sprintf(format, args...),
        HTTPStatus: http.StatusBadRequest,
    }
}

func Unauthenticated(format string, args ...interface{}) *ServiceError {
    return &ServiceError{
        Code:       "TREX-401",
        Reason:     fmt.Sprintf(format, args...),
        HTTPStatus: http.StatusUnauthorized,
    }
}

func Forbidden(format string, args ...interface{}) *ServiceError {
    return &ServiceError{
        Code:       "TREX-403",
        Reason:     fmt.Sprintf(format, args...),
        HTTPStatus: http.StatusForbidden,
    }
}

func NotFound(format string, args ...interface{}) *ServiceError {
    return &ServiceError{
        Code:       "TREX-404",
        Reason:     fmt.Sprintf(format, args...),
        HTTPStatus: http.StatusNotFound,
    }
}

func Conflict(format string, args ...interface{}) *ServiceError {
    return &ServiceError{
        Code:       "TREX-409",
        Reason:     fmt.Sprintf(format, args...),
        HTTPStatus: http.StatusConflict,
    }
}

func UnprocessableEntity(format string, args ...interface{}) *ServiceError {
    return &ServiceError{
        Code:       "TREX-422",
        Reason:     fmt.Sprintf(format, args...),
        HTTPStatus: http.StatusUnprocessableEntity,
    }
}

func TooManyRequests(format string, args ...interface{}) *ServiceError {
    return &ServiceError{
        Code:       "TREX-429",
        Reason:     fmt.Sprintf(format, args...),
        HTTPStatus: http.StatusTooManyRequests,
    }
}

func GeneralError(format string, args ...interface{}) *ServiceError {
    return &ServiceError{
        Code:       "TREX-500",
        Reason:     fmt.Sprintf(format, args...),
        HTTPStatus: http.StatusInternalServerError,
    }
}
```

### Error Response Handling
```go
// File: pkg/errors/handlers.go
func HandleServiceError(r *http.Request, w http.ResponseWriter, err *ServiceError) {
    logger := logger.NewOCMLogger(r.Context())
    
    // Add operation ID for request tracing
    if err.OperationID == "" {
        err.OperationID = getRequestID(r)
    }
    
    // Log error with context
    logger.WithFields(map[string]interface{}{
        "error_code":   err.Code,
        "http_status":  err.HTTPStatus,
        "operation_id": err.OperationID,
        "method":       r.Method,
        "path":         r.URL.Path,
        "user_agent":   r.UserAgent(),
    }).Error(err.Reason)
    
    // Set response headers
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(err.HTTPStatus)
    
    // Write error response
    response := map[string]interface{}{
        "kind":   "Error",
        "id":     err.OperationID,
        "href":   r.URL.Path,
        "code":   err.Code,
        "reason": err.Reason,
    }
    
    json.NewEncoder(w).Encode(response)
}

// Convenience functions for common patterns
func BadRequest(r *http.Request, w http.ResponseWriter, format string, args ...interface{}) {
    err := &ServiceError{
        Code:        "TREX-400",
        Reason:      fmt.Sprintf(format, args...),
        HTTPStatus:  http.StatusBadRequest,
        OperationID: getRequestID(r),
    }
    HandleServiceError(r, w, err)
}

func GeneralError(r *http.Request, w http.ResponseWriter, format string, args ...interface{}) {
    err := &ServiceError{
        Code:        "TREX-500",
        Reason:      fmt.Sprintf(format, args...),
        HTTPStatus:  http.StatusInternalServerError,
        OperationID: getRequestID(r),
    }
    HandleServiceError(r, w, err)
}
```

## Database Error Handling

### GORM Error Translation
```go
// File: pkg/errors/database.go
func TranslateDBError(err error) *ServiceError {
    if err == nil {
        return nil
    }
    
    // GORM record not found
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return NotFound("Resource not found")
    }
    
    // PostgreSQL specific errors
    var pgErr *pq.Error
    if errors.As(err, &pgErr) {
        switch pgErr.Code {
        case "23505": // unique_violation
            return Conflict("Resource already exists")
        case "23503": // foreign_key_violation
            return BadRequest("Invalid reference to related resource")
        case "23502": // not_null_violation
            return BadRequest("Required field missing: %s", pgErr.Column)
        case "23514": // check_violation
            return BadRequest("Value violates constraint: %s", pgErr.Constraint)
        case "42P01": // undefined_table
            return GeneralError("Database schema error")
        default:
            return GeneralError("Database error: %s", pgErr.Message)
        }
    }
    
    // Generic database errors
    if strings.Contains(err.Error(), "duplicate key") {
        return Conflict("Resource already exists")
    }
    
    return GeneralError("Database operation failed: %w", err)
}

// Usage in DAO layer
func (d *dinosaurDao) Create(ctx context.Context, dinosaur *api.Dinosaur) (*api.Dinosaur, *ServiceError) {
    if err := d.g2().WithContext(ctx).Create(dinosaur).Error; err != nil {
        return nil, TranslateDBError(err)
    }
    return dinosaur, nil
}
```

### Transaction Error Handling
```go
func (d *dinosaurDao) CreateWithFossils(ctx context.Context, dinosaur *api.Dinosaur, fossils []api.Fossil) (*api.Dinosaur, *ServiceError) {
    var result *api.Dinosaur
    
    err := d.g2().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        // Create dinosaur
        if err := tx.Create(dinosaur).Error; err != nil {
            return err
        }
        
        // Create associated fossils
        for i := range fossils {
            fossils[i].DinosaurID = dinosaur.ID
            if err := tx.Create(&fossils[i]).Error; err != nil {
                return err
            }
        }
        
        result = dinosaur
        return nil
    })
    
    if err != nil {
        return nil, TranslateDBError(err)
    }
    
    return result, nil
}
```

## Validation Error Patterns

### Request Validation
```go
// File: pkg/handlers/validation.go
type ValidationError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
    Value   interface{} `json:"value,omitempty"`
}

type ValidationErrors []ValidationError

func (v ValidationErrors) Error() string {
    var messages []string
    for _, err := range v {
        messages = append(messages, fmt.Sprintf("%s: %s", err.Field, err.Message))
    }
    return strings.Join(messages, "; ")
}

func ValidateDinosaurRequest(req *api.CreateDinosaurRequest) *ServiceError {
    var validationErrors ValidationErrors
    
    // Required field validation
    if req.Species == "" {
        validationErrors = append(validationErrors, ValidationError{
            Field:   "species",
            Message: "Species is required",
        })
    }
    
    // Length validation
    if len(req.Species) > 255 {
        validationErrors = append(validationErrors, ValidationError{
            Field:   "species",
            Message: "Species name too long (max 255 characters)",
            Value:   len(req.Species),
        })
    }
    
    // Format validation
    if req.Weight != nil && *req.Weight < 0 {
        validationErrors = append(validationErrors, ValidationError{
            Field:   "weight",
            Message: "Weight cannot be negative",
            Value:   *req.Weight,
        })
    }
    
    // Email validation
    if req.ContactEmail != "" && !isValidEmail(req.ContactEmail) {
        validationErrors = append(validationErrors, ValidationError{
            Field:   "contact_email",
            Message: "Invalid email format",
            Value:   req.ContactEmail,
        })
    }
    
    // Date validation
    if req.DiscoveredDate != nil && req.DiscoveredDate.After(time.Now()) {
        validationErrors = append(validationErrors, ValidationError{
            Field:   "discovered_date",
            Message: "Discovery date cannot be in the future",
            Value:   req.DiscoveredDate,
        })
    }
    
    if len(validationErrors) > 0 {
        return &ServiceError{
            Code:       "TREX-VALIDATION",
            Reason:     validationErrors.Error(),
            HTTPStatus: http.StatusBadRequest,
        }
    }
    
    return nil
}
```

### Business Logic Validation
```go
func (s *dinosaurService) ValidateBusinessRules(ctx context.Context, dinosaur *api.Dinosaur) *ServiceError {
    // Check for duplicate species in same location
    existing, err := s.dao.FindBySpeciesAndLocation(ctx, dinosaur.Species, dinosaur.Location)
    if err != nil {
        return TranslateDBError(err)
    }
    if len(existing) > 0 {
        return Conflict("Dinosaur species '%s' already exists in location '%s'", 
            dinosaur.Species, dinosaur.Location)
    }
    
    // Validate weight ranges for species
    if dinosaur.Weight != nil {
        expectedRange := s.getExpectedWeightRange(dinosaur.Species)
        if *dinosaur.Weight < expectedRange.Min || *dinosaur.Weight > expectedRange.Max {
            return UnprocessableEntity("Weight %.2f kg is outside expected range for %s (%.2f - %.2f kg)",
                *dinosaur.Weight, dinosaur.Species, expectedRange.Min, expectedRange.Max)
        }
    }
    
    // Validate temporal constraints
    if dinosaur.Era != "" {
        validEras := []string{"Triassic", "Jurassic", "Cretaceous"}
        if !contains(validEras, dinosaur.Era) {
            return BadRequest("Invalid era: %s. Must be one of: %s", 
                dinosaur.Era, strings.Join(validEras, ", "))
        }
    }
    
    return nil
}
```

## Panic Recovery and Graceful Degradation

### HTTP Panic Recovery Middleware
```go
// File: pkg/handlers/recovery.go
func RecoveryMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if err := recover(); err != nil {
                logger := logger.NewOCMLogger(r.Context())
                
                // Log panic with stack trace
                logger.WithFields(map[string]interface{}{
                    "panic":      err,
                    "method":     r.Method,
                    "path":       r.URL.Path,
                    "user_agent": r.UserAgent(),
                    "stack":      string(debug.Stack()),
                }).Error("HTTP handler panic recovered")
                
                // Return 500 error to client
                if !isResponseStarted(w) {
                    errors.GeneralError(r, w, "Internal server error")
                }
            }
        }()
        
        next.ServeHTTP(w, r)
    })
}

func isResponseStarted(w http.ResponseWriter) bool {
    // Check if headers have been written
    if rw, ok := w.(interface{ Status() int }); ok {
        return rw.Status() != 0
    }
    return false
}
```

### Service-Level Error Recovery
```go
// File: pkg/services/dinosaur.go
func (s *dinosaurService) GetWithRecovery(ctx context.Context, id string) (dinosaur *api.Dinosaur, err *ServiceError) {
    defer func() {
        if r := recover(); r != nil {
            logger := logger.NewOCMLogger(ctx)
            logger.WithFields(map[string]interface{}{
                "panic":       r,
                "dinosaur_id": id,
                "stack":       string(debug.Stack()),
            }).Error("Service method panic recovered")
            
            dinosaur = nil
            err = GeneralError("Service temporarily unavailable")
        }
    }()
    
    // Normal service logic
    return s.dao.Get(ctx, id)
}
```

## Timeout and Context Handling

### Request Timeout Patterns
```go
// File: pkg/handlers/timeout.go
func TimeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ctx, cancel := context.WithTimeout(r.Context(), timeout)
            defer cancel()
            
            // Create a channel to capture the result
            done := make(chan struct{})
            var panicErr interface{}
            
            go func() {
                defer func() {
                    if err := recover(); err != nil {
                        panicErr = err
                    }
                    close(done)
                }()
                
                next.ServeHTTP(w, r.WithContext(ctx))
            }()
            
            select {
            case <-done:
                if panicErr != nil {
                    panic(panicErr) // Re-panic to be caught by recovery middleware
                }
                return
            case <-ctx.Done():
                // Request timed out
                logger := logger.NewOCMLogger(r.Context())
                logger.WithFields(map[string]interface{}{
                    "timeout":    timeout,
                    "method":     r.Method,
                    "path":       r.URL.Path,
                    "user_agent": r.UserAgent(),
                }).Warn("Request timeout")
                
                errors.GeneralError(r, w, "Request timeout")
                return
            }
        })
    }
}
```

### Service Context Handling
```go
func (s *dinosaurService) ProcessLongOperation(ctx context.Context, id string) *ServiceError {
    // Check for cancellation before starting
    select {
    case <-ctx.Done():
        return GeneralError("Request cancelled: %v", ctx.Err())
    default:
    }
    
    // Perform operation with periodic cancellation checks
    for i := 0; i < 100; i++ {
        // Check for cancellation periodically
        select {
        case <-ctx.Done():
            logger := logger.NewOCMLogger(ctx)
            logger.Infof("Long operation cancelled at step %d: %v", i, ctx.Err())
            return GeneralError("Operation cancelled")
        default:
        }
        
        // Do work
        if err := s.processStep(ctx, id, i); err != nil {
            return err
        }
        
        time.Sleep(100 * time.Millisecond) // Simulate work
    }
    
    return nil
}
```

## Error Monitoring and Alerting

### Structured Error Logging
```go
// File: pkg/errors/monitoring.go
func LogError(ctx context.Context, err error, operation string, metadata map[string]interface{}) {
    logger := logger.NewOCMLogger(ctx)
    
    fields := map[string]interface{}{
        "operation":  operation,
        "error_type": fmt.Sprintf("%T", err),
        "timestamp":  time.Now().UTC(),
    }
    
    // Add metadata
    for k, v := range metadata {
        fields[k] = v
    }
    
    // Add request context if available
    if requestID := getRequestID(ctx); requestID != "" {
        fields["request_id"] = requestID
    }
    
    if userID := getUserID(ctx); userID != "" {
        fields["user_id"] = userID
    }
    
    // Determine log level based on error type
    var logLevel string
    if isClientError(err) {
        logLevel = "warn"
        logger.WithFields(fields).Warn(err.Error())
    } else {
        logLevel = "error"
        logger.WithFields(fields).Error(err.Error())
    }
    
    // Send to monitoring system (Sentry, DataDog, etc.)
    if shouldAlert(err) {
        sendAlert(ctx, err, operation, fields)
    }
}

func isClientError(err error) bool {
    if serviceErr, ok := err.(*ServiceError); ok {
        return serviceErr.HTTPStatus >= 400 && serviceErr.HTTPStatus < 500
    }
    return false
}

func shouldAlert(err error) bool {
    if serviceErr, ok := err.(*ServiceError); ok {
        // Alert on server errors, but not client errors
        return serviceErr.HTTPStatus >= 500
    }
    return true // Alert on unexpected error types
}
```
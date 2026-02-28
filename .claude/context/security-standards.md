# TRex Security Standards

## Red Hat Security Requirements

### OCM Authentication Integration
TRex integrates with OpenShift Cluster Manager (OCM) for authentication and authorization:

```go
// File: pkg/auth/middleware.go
func (a *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract bearer token
        authHeader := r.Header.Get("Authorization")
        if authHeader == "" {
            errors.Unauthenticated(r, w, "Authorization header required")
            return
        }
        
        token := strings.TrimPrefix(authHeader, "Bearer ")
        if token == authHeader {
            errors.Unauthenticated(r, w, "Bearer token required")
            return
        }
        
        // Validate token with OCM
        claims, err := a.ocmClient.VerifyToken(r.Context(), token)
        if err != nil {
            // Log token length only - NEVER log actual token
            logger := logger.NewOCMLogger(r.Context())
            logger.Errorf("Token validation failed (token_len=%d): %v", len(token), err)
            errors.Unauthenticated(r, w, "Invalid authentication token")
            return
        }
        
        // Create user context
        user := &api.User{
            ID:       claims.Subject,
            Username: claims.PreferredUsername,
            Email:    claims.Email,
            Roles:    claims.Roles,
        }
        
        // Add user to request context
        ctx := context.WithValue(r.Context(), "user", user)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

### JWT Token Validation
```go
// File: pkg/auth/jwt.go
type JWTValidator struct {
    jwksURL    string
    httpClient *http.Client
    keySet     jwk.Set
    mu         sync.RWMutex
}

func (v *JWTValidator) ValidateToken(ctx context.Context, tokenString string) (*jwt.Token, error) {
    // Parse token without verification first to get key ID
    unverifiedToken, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
    if err != nil {
        return nil, fmt.Errorf("failed to parse token: %w", err)
    }
    
    keyID, ok := unverifiedToken.Header["kid"].(string)
    if !ok {
        return nil, errors.New("token missing key ID")
    }
    
    // Get public key from JWKS
    publicKey, err := v.getPublicKey(ctx, keyID)
    if err != nil {
        return nil, fmt.Errorf("failed to get public key: %w", err)
    }
    
    // Verify token signature and claims
    token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
        // Validate signing method
        if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
            return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
        }
        return publicKey, nil
    })
    
    if err != nil {
        return nil, fmt.Errorf("token validation failed: %w", err)
    }
    
    // Validate standard claims
    claims, ok := token.Claims.(jwt.MapClaims)
    if !ok || !token.Valid {
        return nil, errors.New("invalid token claims")
    }
    
    // Verify expiration
    if exp, ok := claims["exp"].(float64); ok {
        if time.Now().Unix() > int64(exp) {
            return nil, errors.New("token expired")
        }
    }
    
    // Verify issuer
    if iss, ok := claims["iss"].(string); !ok || !v.isValidIssuer(iss) {
        return nil, errors.New("invalid token issuer")
    }
    
    return token, nil
}
```

### Role-Based Access Control (RBAC)
```go
// File: pkg/auth/authorization.go
type Permission struct {
    Resource string
    Action   string
    Scope    string // "own", "organization", "global"
}

func (a *AuthMiddleware) RequirePermission(permission Permission) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            user := GetUserFromContext(r.Context())
            if user == nil {
                errors.Unauthenticated(r, w, "Authentication required")
                return
            }
            
            // Check if user has required permission
            if !a.hasPermission(user, permission, r) {
                logger := logger.NewOCMLogger(r.Context())
                logger.Warnf("User %s denied access to %s:%s (scope: %s)", 
                    user.Username, permission.Resource, permission.Action, permission.Scope)
                errors.Forbidden(r, w, "Insufficient permissions")
                return
            }
            
            next.ServeHTTP(w, r)
        })
    }
}

func (a *AuthMiddleware) hasPermission(user *api.User, perm Permission, r *http.Request) bool {
    // Global admin override
    if user.HasRole("trex-admin") {
        return true
    }
    
    // Check resource-specific permissions
    switch perm.Resource {
    case "dinosaurs":
        return a.checkDinosaurPermission(user, perm, r)
    case "fossils":
        return a.checkFossilPermission(user, perm, r)
    default:
        return false
    }
}

func (a *AuthMiddleware) checkDinosaurPermission(user *api.User, perm Permission, r *http.Request) bool {
    switch perm.Action {
    case "create":
        return user.HasRole("dinosaur-creator") || user.HasRole("dinosaur-admin")
    case "read":
        if perm.Scope == "own" {
            dinosaurID := mux.Vars(r)["id"]
            return a.isDinosaurOwner(user.ID, dinosaurID)
        }
        return user.HasRole("dinosaur-reader") || user.HasRole("dinosaur-admin")
    case "update", "delete":
        if perm.Scope == "own" {
            dinosaurID := mux.Vars(r)["id"]
            return a.isDinosaurOwner(user.ID, dinosaurID)
        }
        return user.HasRole("dinosaur-admin")
    default:
        return false
    }
}
```

## Input Validation and Sanitization

### Request Validation Patterns
```go
// File: pkg/handlers/validation.go
func ValidateCreateDinosaurRequest(req *api.CreateDinosaurRequest) *errors.ServiceError {
    if req.Species == "" {
        return errors.BadRequest("Species is required")
    }
    
    // Prevent code injection in names
    if containsSQLKeywords(req.Species) {
        return errors.BadRequest("Species contains invalid characters")
    }
    
    // Length validation
    if len(req.Species) > 255 {
        return errors.BadRequest("Species name too long (max 255 characters)")
    }
    
    // Format validation using regex
    if !isValidSpeciesName(req.Species) {
        return errors.BadRequest("Species name contains invalid characters")
    }
    
    // Validate nested objects
    if req.Location != nil {
        if err := ValidateLocation(req.Location); err != nil {
            return errors.BadRequest("Invalid location: %s", err.Error())
        }
    }
    
    // Validate arrays
    if len(req.Tags) > 10 {
        return errors.BadRequest("Too many tags (max 10)")
    }
    for _, tag := range req.Tags {
        if !isValidTag(tag) {
            return errors.BadRequest("Invalid tag format: %s", tag)
        }
    }
    
    return nil
}

func isValidSpeciesName(name string) bool {
    // Only allow letters, numbers, spaces, and basic punctuation
    validName := regexp.MustCompile(`^[a-zA-Z0-9\s\-_\.]+$`)
    return validName.MatchString(name)
}

func containsSQLKeywords(input string) bool {
    sqlKeywords := []string{"SELECT", "INSERT", "UPDATE", "DELETE", "DROP", "UNION", "EXEC"}
    upperInput := strings.ToUpper(input)
    for _, keyword := range sqlKeywords {
        if strings.Contains(upperInput, keyword) {
            return true
        }
    }
    return false
}

func isValidUUID(id string) bool {
    _, err := uuid.Parse(id)
    return err == nil
}
```

### SQL Injection Prevention
```go
// File: pkg/dao/secure_queries.go

// GOOD: Always use parameterized queries
func (d *dinosaurDao) SearchBySpecies(ctx context.Context, species string) ([]api.Dinosaur, error) {
    var dinosaurs []api.Dinosaur
    
    // GORM automatically parameterizes queries
    err := d.g2().WithContext(ctx).
        Where("species ILIKE ?", "%"+species+"%").
        Find(&dinosaurs).Error
        
    return dinosaurs, err
}

// GOOD: Raw queries with parameters
func (d *dinosaurDao) GetDinosaurStats(ctx context.Context, speciesFilter string) (*api.DinosaurStats, error) {
    var stats api.DinosaurStats
    
    query := `
        SELECT 
            COUNT(*) as total,
            AVG(weight) as avg_weight,
            MAX(discovered_date) as latest_discovery
        FROM dinosaurs 
        WHERE species = ? AND deleted_at IS NULL
    `
    
    err := d.g2().WithContext(ctx).Raw(query, speciesFilter).Scan(&stats).Error
    return &stats, err
}

// BAD: Never concatenate user input into SQL
func (d *dinosaurDao) SearchBySpeciesBad(ctx context.Context, species string) ([]api.Dinosaur, error) {
    var dinosaurs []api.Dinosaur
    
    // VULNERABLE to SQL injection - NEVER DO THIS
    query := fmt.Sprintf("SELECT * FROM dinosaurs WHERE species LIKE '%%%s%%'", species)
    err := d.g2().WithContext(ctx).Raw(query).Scan(&dinosaurs).Error
    
    return dinosaurs, err
}
```

### File Upload Security
```go
// File: pkg/handlers/upload.go
func (h *DinosaurHandler) UploadImage(w http.ResponseWriter, r *http.Request) {
    // Limit upload size (10MB)
    r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024)
    
    // Parse multipart form
    if err := r.ParseMultipartForm(10 * 1024 * 1024); err != nil {
        errors.BadRequest(r, w, "File too large")
        return
    }
    
    file, header, err := r.FormFile("image")
    if err != nil {
        errors.BadRequest(r, w, "No file uploaded")
        return
    }
    defer file.Close()
    
    // Validate file type
    if !isValidImageType(header.Filename) {
        errors.BadRequest(r, w, "Invalid file type. Only JPEG and PNG allowed")
        return
    }
    
    // Read first 512 bytes to detect actual file type
    buffer := make([]byte, 512)
    if _, err := file.Read(buffer); err != nil {
        errors.GeneralError(r, w, "Failed to read file")
        return
    }
    
    // Verify MIME type matches extension
    mimeType := http.DetectContentType(buffer)
    if !isAllowedMimeType(mimeType) {
        errors.BadRequest(r, w, "File content does not match extension")
        return
    }
    
    // Reset file pointer
    if _, err := file.Seek(0, 0); err != nil {
        errors.GeneralError(r, w, "Failed to process file")
        return
    }
    
    // Generate safe filename
    safeFilename := generateSafeFilename(header.Filename)
    
    // Save file to secure location
    if err := saveImageFile(file, safeFilename); err != nil {
        errors.GeneralError(r, w, "Failed to save file")
        return
    }
    
    // Return success response
    response := map[string]string{
        "filename": safeFilename,
        "url":      fmt.Sprintf("/api/rh-trex/v1/images/%s", safeFilename),
    }
    
    presenters.Present(w, response)
}

func isValidImageType(filename string) bool {
    ext := strings.ToLower(filepath.Ext(filename))
    return ext == ".jpg" || ext == ".jpeg" || ext == ".png"
}

func isAllowedMimeType(mimeType string) bool {
    allowed := []string{"image/jpeg", "image/png"}
    for _, allowed := range allowed {
        if mimeType == allowed {
            return true
        }
    }
    return false
}

func generateSafeFilename(original string) string {
    // Remove path components
    filename := filepath.Base(original)
    
    // Generate UUID for uniqueness
    id := uuid.New().String()
    ext := filepath.Ext(filename)
    
    return fmt.Sprintf("%s%s", id, ext)
}
```

## Secret Management

### Never Log Secrets
```go
// GOOD: Log token length only
func (a *AuthService) ProcessToken(ctx context.Context, token string) error {
    logger := logger.NewOCMLogger(ctx)
    logger.Infof("Processing authentication token (len=%d)", len(token))
    
    // Process token...
    if err := a.validateToken(token); err != nil {
        logger.Errorf("Token validation failed (len=%d): %v", len(token), err)
        return err
    }
    
    logger.Infof("Token validation successful (len=%d)", len(token))
    return nil
}

// BAD: NEVER log actual secrets
func (a *AuthService) ProcessTokenBad(ctx context.Context, token string) error {
    logger := logger.NewOCMLogger(ctx)
    
    // SECURITY VIOLATION - secrets in logs
    logger.Infof("Processing token: %s", token)
    
    return nil
}

// GOOD: Redact sensitive data in structs
func (u *User) String() string {
    return fmt.Sprintf("User{ID: %s, Username: %s, Email: %s, Token: [REDACTED]}", 
        u.ID, u.Username, u.Email)
}

// GOOD: Use structured logging with redacted fields
func (a *AuthService) LogUserAction(ctx context.Context, user *api.User, action string) {
    logger := logger.NewOCMLogger(ctx)
    logger.WithFields(map[string]interface{}{
        "user_id":   user.ID,
        "username":  user.Username,
        "action":    action,
        "token_len": len(user.Token),
        // Never include: user.Token, user.Password, user.APIKey
    }).Info("User action performed")
}
```

### Environment Variable Security
```go
// File: pkg/config/secrets.go
type SecretsConfig struct {
    JWTSigningKey    string `env:"JWT_SIGNING_KEY,required"`
    DatabasePassword string `env:"DB_PASSWORD,required"`
    OCMClientSecret  string `env:"OCM_CLIENT_SECRET,required"`
    SentryDSN        string `env:"SENTRY_DSN"`
}

// Safe string representation for logging
func (s SecretsConfig) String() string {
    return fmt.Sprintf("SecretsConfig{JWTSigningKey: [%d chars], DatabasePassword: [%d chars], OCMClientSecret: [%d chars]}",
        len(s.JWTSigningKey), len(s.DatabasePassword), len(s.OCMClientSecret))
}

// Load secrets from files (Kubernetes secrets pattern)
func LoadSecretsFromFiles(config *SecretsConfig) error {
    secretFiles := map[string]*string{
        "secrets/jwt.key":         &config.JWTSigningKey,
        "secrets/db.password":     &config.DatabasePassword,
        "secrets/ocm.secret":      &config.OCMClientSecret,
    }
    
    for filePath, target := range secretFiles {
        if content, err := os.ReadFile(filePath); err == nil {
            *target = strings.TrimSpace(string(content))
        }
    }
    
    return nil
}
```

## Security Headers and CORS

### Secure HTTP Headers
```go
// File: pkg/handlers/security_middleware.go
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Prevent XSS attacks
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("X-XSS-Protection", "1; mode=block")
        
        // HSTS for HTTPS
        if r.TLS != nil {
            w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        }
        
        // Content Security Policy
        w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'")
        
        // Referrer Policy
        w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
        
        next.ServeHTTP(w, r)
    })
}
```

### CORS Configuration
```go
// File: pkg/handlers/cors.go
func NewCORSHandler(allowedOrigins []string) func(http.Handler) http.Handler {
    return cors.New(cors.Options{
        AllowedOrigins:   allowedOrigins,
        AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
        AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Requested-With"},
        ExposedHeaders:   []string{"X-Total-Count", "X-Page-Size"},
        AllowCredentials: true,
        MaxAge:           300, // 5 minutes
    }).Handler
}
```

## Rate Limiting and DDoS Protection

### Request Rate Limiting
```go
// File: pkg/handlers/rate_limiting.go
func NewRateLimiter(requestsPerMinute int) func(http.Handler) http.Handler {
    limiter := rate.NewLimiter(rate.Limit(requestsPerMinute), requestsPerMinute*2)
    
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if !limiter.Allow() {
                errors.TooManyRequests(r, w, "Rate limit exceeded")
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}

// Per-user rate limiting
func NewPerUserRateLimiter(requestsPerMinute int) func(http.Handler) http.Handler {
    limiters := sync.Map{}
    
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            user := GetUserFromContext(r.Context())
            if user == nil {
                errors.Unauthenticated(r, w, "Authentication required")
                return
            }
            
            // Get or create limiter for this user
            limiterInterface, _ := limiters.LoadOrStore(user.ID, 
                rate.NewLimiter(rate.Limit(requestsPerMinute), requestsPerMinute*2))
            limiter := limiterInterface.(*rate.Limiter)
            
            if !limiter.Allow() {
                errors.TooManyRequests(r, w, "User rate limit exceeded")
                return
            }
            
            next.ServeHTTP(w, r)
        })
    }
}
```

## Audit Logging

### Security Event Logging
```go
// File: pkg/audit/security_events.go
type SecurityEvent struct {
    Timestamp    time.Time `json:"timestamp"`
    UserID       string    `json:"user_id"`
    Username     string    `json:"username"`
    Action       string    `json:"action"`
    Resource     string    `json:"resource"`
    ResourceID   string    `json:"resource_id,omitempty"`
    IPAddress    string    `json:"ip_address"`
    UserAgent    string    `json:"user_agent"`
    Success      bool      `json:"success"`
    ErrorMessage string    `json:"error_message,omitempty"`
    RequestID    string    `json:"request_id"`
}

func LogSecurityEvent(ctx context.Context, event SecurityEvent) {
    logger := logger.NewOCMLogger(ctx)
    
    // Add request ID from context
    if requestID := GetRequestIDFromContext(ctx); requestID != "" {
        event.RequestID = requestID
    }
    
    event.Timestamp = time.Now().UTC()
    
    // Log as structured JSON for security monitoring
    logger.WithFields(map[string]interface{}{
        "event_type":    "security",
        "user_id":       event.UserID,
        "username":      event.Username,
        "action":        event.Action,
        "resource":      event.Resource,
        "resource_id":   event.ResourceID,
        "ip_address":    event.IPAddress,
        "success":       event.Success,
        "error_message": event.ErrorMessage,
        "request_id":    event.RequestID,
    }).Info("Security event")
}

// Usage in handlers
func (h *DinosaurHandler) Delete(w http.ResponseWriter, r *http.Request) {
    user := GetUserFromContext(r.Context())
    dinosaurID := mux.Vars(r)["id"]
    
    err := h.service.Delete(r.Context(), dinosaurID)
    
    // Log security event
    LogSecurityEvent(r.Context(), SecurityEvent{
        UserID:     user.ID,
        Username:   user.Username,
        Action:     "delete",
        Resource:   "dinosaur",
        ResourceID: dinosaurID,
        IPAddress:  GetClientIP(r),
        UserAgent:  r.UserAgent(),
        Success:    err == nil,
        ErrorMessage: func() string {
            if err != nil {
                return err.Error()
            }
            return ""
        }(),
    })
    
    if err != nil {
        errors.HandleServiceError(r, w, err)
        return
    }
    
    w.WriteHeader(http.StatusNoContent)
}
```
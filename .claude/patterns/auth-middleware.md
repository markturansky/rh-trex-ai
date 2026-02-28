# TRex Authentication and Authorization Middleware Patterns

## OCM Integration Patterns

### JWT Token Validation Middleware
```go
// File: pkg/auth/jwt_middleware.go
type JWTAuthMiddleware struct {
    jwksURL     string
    validator   *JWTValidator
    userService api.UserService
    cache       *jwk.Cache
}

func NewJWTAuthMiddleware(config *config.AuthConfig, userService api.UserService) *JWTAuthMiddleware {
    cache := jwk.NewCache(context.Background())
    cache.Register(config.JWKSURL, jwk.WithMinRefreshInterval(5*time.Minute))
    
    return &JWTAuthMiddleware{
        jwksURL:     config.JWKSURL,
        validator:   NewJWTValidator(config),
        userService: userService,
        cache:       cache,
    }
}

func (m *JWTAuthMiddleware) Authenticate(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        logger := logger.NewOCMLogger(r.Context())
        
        // Extract bearer token from Authorization header
        authHeader := r.Header.Get("Authorization")
        if authHeader == "" {
            logger.Warn("Missing Authorization header")
            errors.Unauthenticated(r, w, "Authorization header required")
            return
        }
        
        const bearerPrefix = "Bearer "
        if !strings.HasPrefix(authHeader, bearerPrefix) {
            logger.Warn("Invalid Authorization header format")
            errors.Unauthenticated(r, w, "Bearer token required")
            return
        }
        
        tokenString := strings.TrimPrefix(authHeader, bearerPrefix)
        if tokenString == "" {
            logger.Warn("Empty bearer token")
            errors.Unauthenticated(r, w, "Bearer token cannot be empty")
            return
        }
        
        // Validate JWT token
        token, err := m.validator.ValidateToken(r.Context(), tokenString)
        if err != nil {
            logger.WithFields(map[string]interface{}{
                "token_len": len(tokenString),
                "error":     err.Error(),
            }).Warn("JWT token validation failed")
            errors.Unauthenticated(r, w, "Invalid authentication token")
            return
        }
        
        // Extract claims
        claims, ok := token.Claims.(jwt.MapClaims)
        if !ok {
            logger.Error("Invalid JWT claims format")
            errors.Unauthenticated(r, w, "Invalid token claims")
            return
        }
        
        // Create user from claims
        user, err := m.createUserFromClaims(r.Context(), claims)
        if err != nil {
            logger.WithFields(map[string]interface{}{
                "subject": claims["sub"],
                "error":   err.Error(),
            }).Error("Failed to create user from token claims")
            errors.Unauthenticated(r, w, "Failed to process user information")
            return
        }
        
        // Add user and token to context
        ctx := context.WithValue(r.Context(), UserContextKey, user)
        ctx = context.WithValue(ctx, TokenContextKey, tokenString)
        
        logger.WithFields(map[string]interface{}{
            "user_id":  user.ID,
            "username": user.Username,
        }).Info("User authenticated successfully")
        
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func (m *JWTAuthMiddleware) createUserFromClaims(ctx context.Context, claims jwt.MapClaims) (*api.User, error) {
    // Extract standard claims
    subject, ok := claims["sub"].(string)
    if !ok || subject == "" {
        return nil, errors.New("missing or invalid subject claim")
    }
    
    username, _ := claims["preferred_username"].(string)
    email, _ := claims["email"].(string)
    name, _ := claims["name"].(string)
    
    // Extract organization information
    orgID, _ := claims["org_id"].(string)
    
    // Extract roles from custom claims
    var roles []string
    if rolesInterface, ok := claims["realm_access"].(map[string]interface{}); ok {
        if rolesArray, ok := rolesInterface["roles"].([]interface{}); ok {
            for _, role := range rolesArray {
                if roleStr, ok := role.(string); ok {
                    roles = append(roles, roleStr)
                }
            }
        }
    }
    
    return &api.User{
        ID:             subject,
        Username:       username,
        Email:          email,
        Name:           name,
        OrganizationID: orgID,
        Roles:          roles,
        CreatedAt:      time.Now(),
    }, nil
}
```

### Role-Based Authorization Middleware
```go
// File: pkg/auth/rbac_middleware.go
type Permission struct {
    Resource string   // "dinosaurs", "fossils", "users"
    Action   string   // "create", "read", "update", "delete", "list"
    Scope    string   // "own", "organization", "global"
}

type AuthorizationMiddleware struct {
    permissions map[string][]Permission // role -> permissions mapping
    policies    map[string]PolicyFunc   // resource -> policy function
}

type PolicyFunc func(user *api.User, resource string, action string, context map[string]interface{}) bool

func NewAuthorizationMiddleware() *AuthorizationMiddleware {
    return &AuthorizationMiddleware{
        permissions: make(map[string][]Permission),
        policies:    make(map[string]PolicyFunc),
    }
}

func (m *AuthorizationMiddleware) RequirePermission(resource, action string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            user := GetUserFromContext(r.Context())
            if user == nil {
                errors.Unauthenticated(r, w, "Authentication required")
                return
            }
            
            // Check if user has permission
            if !m.hasPermission(user, resource, action, r) {
                logger := logger.NewOCMLogger(r.Context())
                logger.WithFields(map[string]interface{}{
                    "user_id":  user.ID,
                    "username": user.Username,
                    "resource": resource,
                    "action":   action,
                    "path":     r.URL.Path,
                }).Warn("Authorization failed - insufficient permissions")
                
                errors.Forbidden(r, w, "Insufficient permissions to perform this action")
                return
            }
            
            next.ServeHTTP(w, r)
        })
    }
}

func (m *AuthorizationMiddleware) hasPermission(user *api.User, resource, action string, r *http.Request) bool {
    // Global admin override
    if user.HasRole("trex-admin") {
        return true
    }
    
    // Check role-based permissions
    for _, role := range user.Roles {
        permissions := m.permissions[role]
        for _, perm := range permissions {
            if perm.Resource == resource && perm.Action == action {
                // Check scope-specific authorization
                return m.checkScope(user, perm, r)
            }
            
            // Wildcard permissions
            if perm.Resource == "*" || perm.Action == "*" {
                return m.checkScope(user, perm, r)
            }
        }
    }
    
    // Check resource-specific policies
    if policy, exists := m.policies[resource]; exists {
        context := m.extractContext(r)
        return policy(user, resource, action, context)
    }
    
    return false
}

func (m *AuthorizationMiddleware) checkScope(user *api.User, perm Permission, r *http.Request) bool {
    switch perm.Scope {
    case "global":
        return true
        
    case "organization":
        // Check if user is accessing resources within their organization
        if resourceOrgID := r.Header.Get("X-Organization-ID"); resourceOrgID != "" {
            return resourceOrgID == user.OrganizationID
        }
        return true
        
    case "own":
        // Check if user is accessing their own resources
        resourceID := mux.Vars(r)["id"]
        if resourceID != "" {
            return m.isResourceOwner(user, perm.Resource, resourceID)
        }
        return true
        
    default:
        return false
    }
}

func (m *AuthorizationMiddleware) isResourceOwner(user *api.User, resource, resourceID string) bool {
    // This would typically query the database to check ownership
    // Implementation depends on specific resource types
    switch resource {
    case "dinosaurs":
        // Check if user created this dinosaur
        return m.checkDinosaurOwnership(user.ID, resourceID)
    case "fossils":
        // Check if user owns the fossil or the related dinosaur
        return m.checkFossilOwnership(user.ID, resourceID)
    default:
        return false
    }
}
```

### OCM Client Integration
```go
// File: pkg/auth/ocm_client.go
type OCMClient struct {
    baseURL      string
    clientID     string
    clientSecret string
    httpClient   *http.Client
    tokenCache   *TokenCache
}

type OCMTokenResponse struct {
    AccessToken  string `json:"access_token"`
    TokenType    string `json:"token_type"`
    ExpiresIn    int    `json:"expires_in"`
    RefreshToken string `json:"refresh_token"`
    Scope        string `json:"scope"`
}

type TokenCache struct {
    mu     sync.RWMutex
    tokens map[string]*CachedToken
}

type CachedToken struct {
    Token     *OCMTokenResponse
    ExpiresAt time.Time
}

func NewOCMClient(config *config.OCMConfig) *OCMClient {
    return &OCMClient{
        baseURL:      config.BaseURL,
        clientID:     config.ClientID,
        clientSecret: config.ClientSecret,
        httpClient: &http.Client{
            Timeout: 30 * time.Second,
        },
        tokenCache: &TokenCache{
            tokens: make(map[string]*CachedToken),
        },
    }
}

func (c *OCMClient) VerifyToken(ctx context.Context, token string) (*jwt.MapClaims, error) {
    // First, try to validate token locally if we have the public key
    if claims, err := c.validateTokenLocally(token); err == nil {
        return claims, nil
    }
    
    // If local validation fails, verify with OCM service
    return c.verifyTokenWithOCM(ctx, token)
}

func (c *OCMClient) validateTokenLocally(tokenString string) (*jwt.MapClaims, error) {
    // Parse token without verification to get key ID
    token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
    if err != nil {
        return nil, err
    }
    
    keyID, ok := token.Header["kid"].(string)
    if !ok {
        return nil, errors.New("token missing key ID")
    }
    
    // Get public key from JWKS endpoint
    publicKey, err := c.getPublicKey(keyID)
    if err != nil {
        return nil, err
    }
    
    // Verify token
    parsedToken, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
        if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
            return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
        }
        return publicKey, nil
    })
    
    if err != nil {
        return nil, err
    }
    
    claims, ok := parsedToken.Claims.(jwt.MapClaims)
    if !ok || !parsedToken.Valid {
        return nil, errors.New("invalid token claims")
    }
    
    return &claims, nil
}

func (c *OCMClient) verifyTokenWithOCM(ctx context.Context, token string) (*jwt.MapClaims, error) {
    url := fmt.Sprintf("%s/api/accounts_mgmt/v1/token_authorization", c.baseURL)
    
    req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
    if err != nil {
        return nil, err
    }
    
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode == http.StatusUnauthorized {
        return nil, errors.New("token unauthorized")
    }
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("OCM API returned status %d", resp.StatusCode)
    }
    
    var authResp struct {
        Account struct {
            ID       string `json:"id"`
            Username string `json:"username"`
            Email    string `json:"email"`
            OrgID    string `json:"organization_id"`
        } `json:"account"`
    }
    
    if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
        return nil, err
    }
    
    // Convert OCM response to JWT claims format
    claims := jwt.MapClaims{
        "sub":                authResp.Account.ID,
        "preferred_username": authResp.Account.Username,
        "email":             authResp.Account.Email,
        "org_id":            authResp.Account.OrgID,
        "iss":               c.baseURL,
        "exp":               time.Now().Add(time.Hour).Unix(),
    }
    
    return &claims, nil
}
```

### Context Helper Functions
```go
// File: pkg/auth/context.go
type contextKey string

const (
    UserContextKey  contextKey = "user"
    TokenContextKey contextKey = "token"
    RequestIDKey    contextKey = "request_id"
)

func GetUserFromContext(ctx context.Context) *api.User {
    if user, ok := ctx.Value(UserContextKey).(*api.User); ok {
        return user
    }
    return nil
}

func GetTokenFromContext(ctx context.Context) string {
    if token, ok := ctx.Value(TokenContextKey).(string); ok {
        return token
    }
    return ""
}

func GetRequestIDFromContext(ctx context.Context) string {
    if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
        return requestID
    }
    return ""
}

func WithUser(ctx context.Context, user *api.User) context.Context {
    return context.WithValue(ctx, UserContextKey, user)
}

func WithToken(ctx context.Context, token string) context.Context {
    return context.WithValue(ctx, TokenContextKey, token)
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
    return context.WithValue(ctx, RequestIDKey, requestID)
}

// Helper for checking user roles
func (u *User) HasRole(role string) bool {
    for _, userRole := range u.Roles {
        if userRole == role {
            return true
        }
    }
    return false
}

func (u *User) HasAnyRole(roles ...string) bool {
    for _, role := range roles {
        if u.HasRole(role) {
            return true
        }
    }
    return false
}

func (u *User) IsAdmin() bool {
    return u.HasAnyRole("trex-admin", "admin", "cluster-admin")
}
```

## Resource-Specific Authorization

### Dinosaur Resource Authorization
```go
// File: pkg/auth/dinosaur_policy.go
func DinosaurAuthorizationPolicy(user *api.User, resource, action string, context map[string]interface{}) bool {
    switch action {
    case "create":
        // Any authenticated user can create dinosaurs
        return true
        
    case "read", "list":
        // Users can read all dinosaurs unless restricted
        return true
        
    case "update":
        // Users can update their own dinosaurs or if they're moderators
        if user.HasRole("dinosaur-moderator") {
            return true
        }
        
        if dinosaurID, ok := context["dinosaur_id"].(string); ok {
            return isDinosaurOwner(user.ID, dinosaurID)
        }
        return false
        
    case "delete":
        // Only admins or owners can delete dinosaurs
        if user.HasRole("dinosaur-admin") {
            return true
        }
        
        if dinosaurID, ok := context["dinosaur_id"].(string); ok {
            return isDinosaurOwner(user.ID, dinosaurID)
        }
        return false
        
    default:
        return false
    }
}

func FossilAuthorizationPolicy(user *api.User, resource, action string, context map[string]interface{}) bool {
    switch action {
    case "create":
        // Must own the dinosaur to add fossils
        if dinosaurID, ok := context["dinosaur_id"].(string); ok {
            return isDinosaurOwner(user.ID, dinosaurID)
        }
        return false
        
    case "read", "list":
        // Can read fossils if you can read the dinosaur
        return true
        
    case "update", "delete":
        // Must own the fossil or the related dinosaur
        if fossilID, ok := context["fossil_id"].(string); ok {
            return isFossilOwner(user.ID, fossilID)
        }
        return false
        
    default:
        return false
    }
}
```

### Middleware Chain Configuration
```go
// File: pkg/handlers/middleware_chain.go
func SetupAuthenticationMiddleware(config *config.Config, userService api.UserService) []func(http.Handler) http.Handler {
    var middleware []func(http.Handler) http.Handler
    
    // Request ID middleware (for tracing)
    middleware = append(middleware, RequestIDMiddleware())
    
    // CORS middleware
    middleware = append(middleware, CORSMiddleware(config.CORS.AllowedOrigins))
    
    // Security headers
    middleware = append(middleware, SecurityHeadersMiddleware())
    
    // Rate limiting
    middleware = append(middleware, RateLimitingMiddleware(config.RateLimit.RequestsPerMinute))
    
    // Authentication
    if config.Auth.EnableJWT {
        jwtMiddleware := NewJWTAuthMiddleware(&config.Auth, userService)
        middleware = append(middleware, jwtMiddleware.Authenticate)
    }
    
    // Recovery (should be last)
    middleware = append(middleware, RecoveryMiddleware())
    
    return middleware
}

func ApplyMiddleware(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
    for i := len(middlewares) - 1; i >= 0; i-- {
        handler = middlewares[i](handler)
    }
    return handler
}

// Usage in router setup
func (s *APIServer) setupRoutes() {
    // Public endpoints (no auth required)
    s.router.HandleFunc("/health", s.healthHandler).Methods("GET")
    s.router.HandleFunc("/metrics", s.metricsHandler).Methods("GET")
    
    // Protected API endpoints
    api := s.router.PathPrefix("/api/rh-trex/v1").Subrouter()
    
    // Apply authentication middleware to all API routes
    authMiddleware := SetupAuthenticationMiddleware(s.config, s.userService)
    api.Use(authMiddleware...)
    
    // Dinosaur endpoints with specific authorization
    authz := NewAuthorizationMiddleware()
    authz.RegisterPolicy("dinosaurs", DinosaurAuthorizationPolicy)
    authz.RegisterPolicy("fossils", FossilAuthorizationPolicy)
    
    dinosaurRouter := api.PathPrefix("/dinosaurs").Subrouter()
    dinosaurRouter.Use(authz.RequirePermission("dinosaurs", "read"))
    dinosaurRouter.HandleFunc("", s.dinosaurHandler.List).Methods("GET")
    dinosaurRouter.HandleFunc("", s.dinosaurHandler.Create).Methods("POST")
    
    dinosaurItemRouter := dinosaurRouter.PathPrefix("/{id}").Subrouter()
    dinosaurItemRouter.HandleFunc("", s.dinosaurHandler.Get).Methods("GET")
    dinosaurItemRouter.HandleFunc("", s.dinosaurHandler.Update).Methods("PATCH")
    dinosaurItemRouter.HandleFunc("", s.dinosaurHandler.Delete).Methods("DELETE")
}
```

## gRPC Authentication

### gRPC Auth Interceptor
```go
// File: pkg/server/grpc_auth_interceptor.go
func NewAuthInterceptor(jwtValidator *auth.JWTValidator) grpc.UnaryServerInterceptor {
    return func(
        ctx context.Context,
        req interface{},
        info *grpc.UnaryServerInfo,
        handler grpc.UnaryHandler,
    ) (interface{}, error) {
        // Skip auth for health checks
        if info.FullMethod == "/grpc.health.v1.Health/Check" {
            return handler(ctx, req)
        }
        
        // Extract token from metadata
        md, ok := metadata.FromIncomingContext(ctx)
        if !ok {
            return nil, status.Error(codes.Unauthenticated, "missing metadata")
        }
        
        tokens := md.Get("authorization")
        if len(tokens) == 0 {
            return nil, status.Error(codes.Unauthenticated, "missing authorization header")
        }
        
        token := tokens[0]
        if !strings.HasPrefix(token, "Bearer ") {
            return nil, status.Error(codes.Unauthenticated, "invalid authorization header format")
        }
        
        tokenString := strings.TrimPrefix(token, "Bearer ")
        
        // Validate token
        jwtToken, err := jwtValidator.ValidateToken(ctx, tokenString)
        if err != nil {
            return nil, status.Error(codes.Unauthenticated, "invalid token")
        }
        
        // Extract user from claims
        claims, ok := jwtToken.Claims.(jwt.MapClaims)
        if !ok {
            return nil, status.Error(codes.Unauthenticated, "invalid token claims")
        }
        
        user := &api.User{
            ID:       claims["sub"].(string),
            Username: claims["preferred_username"].(string),
            Email:    claims["email"].(string),
        }
        
        // Add user to context
        newCtx := auth.WithUser(ctx, user)
        
        return handler(newCtx, req)
    }
}

// Streaming interceptor
func NewStreamAuthInterceptor(jwtValidator *auth.JWTValidator) grpc.StreamServerInterceptor {
    return func(
        srv interface{},
        ss grpc.ServerStream,
        info *grpc.StreamServerInfo,
        handler grpc.StreamHandler,
    ) error {
        // Similar authentication logic as unary interceptor
        ctx := ss.Context()
        
        md, ok := metadata.FromIncomingContext(ctx)
        if !ok {
            return status.Error(codes.Unauthenticated, "missing metadata")
        }
        
        tokens := md.Get("authorization")
        if len(tokens) == 0 {
            return status.Error(codes.Unauthenticated, "missing authorization header")
        }
        
        token := strings.TrimPrefix(tokens[0], "Bearer ")
        
        jwtToken, err := jwtValidator.ValidateToken(ctx, token)
        if err != nil {
            return status.Error(codes.Unauthenticated, "invalid token")
        }
        
        claims, ok := jwtToken.Claims.(jwt.MapClaims)
        if !ok {
            return status.Error(codes.Unauthenticated, "invalid token claims")
        }
        
        user := &api.User{
            ID:       claims["sub"].(string),
            Username: claims["preferred_username"].(string),
            Email:    claims["email"].(string),
        }
        
        // Wrap stream with authenticated context
        wrappedStream := &AuthenticatedStream{
            ServerStream: ss,
            ctx:          auth.WithUser(ctx, user),
        }
        
        return handler(srv, wrappedStream)
    }
}

type AuthenticatedStream struct {
    grpc.ServerStream
    ctx context.Context
}

func (s *AuthenticatedStream) Context() context.Context {
    return s.ctx
}
```
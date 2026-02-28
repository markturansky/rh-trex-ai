# TRex Testing Patterns

## Test Organization Structure

### Test Directory Layout
```
test/
├── integration/           # End-to-end API tests
│   ├── dinosaurs_test.go
│   ├── fossils_test.go
│   ├── auth_test.go
│   └── helpers/
├── factories/            # Test data factories
│   ├── dinosaurs.go
│   ├── fossils.go
│   └── users.go
├── mocks/               # Generated mocks
│   ├── dinosaur_service.go
│   └── fossil_dao.go
└── testdata/           # Static test files
    ├── valid_dinosaur.json
    └── test_image.jpg
```

### Unit Test Patterns
```go
// File: pkg/services/dinosaur_test.go
package services

import (
    "context"
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/require"
    
    "github.com/openshift-online/rh-trex-ai/pkg/api"
    "github.com/openshift-online/rh-trex-ai/pkg/dao/mocks"
    "github.com/openshift-online/rh-trex-ai/pkg/errors"
    "github.com/openshift-online/rh-trex-ai/test/factories"
)

func TestDinosaurService_Create(t *testing.T) {
    tests := []struct {
        name           string
        input          *api.Dinosaur
        mockSetup      func(*mocks.DinosaurDao)
        expectedResult *api.Dinosaur
        expectedError  string
    }{
        {
            name:  "successful creation",
            input: factories.BuildDinosaur(factories.DinosaurConfig{Species: "T-Rex"}),
            mockSetup: func(mockDao *mocks.DinosaurDao) {
                mockDao.On("Create", mock.Anything, mock.AnythingOfType("*api.Dinosaur")).
                    Return(factories.BuildDinosaur(factories.DinosaurConfig{
                        ID:      "test-id-123",
                        Species: "T-Rex",
                    }), nil)
            },
            expectedResult: factories.BuildDinosaur(factories.DinosaurConfig{
                ID:      "test-id-123", 
                Species: "T-Rex",
            }),
        },
        {
            name:  "validation failure - empty species",
            input: factories.BuildDinosaur(factories.DinosaurConfig{Species: ""}),
            mockSetup: func(mockDao *mocks.DinosaurDao) {
                // No DAO calls expected
            },
            expectedError: "Species is required",
        },
        {
            name:  "database error",
            input: factories.BuildDinosaur(factories.DinosaurConfig{Species: "T-Rex"}),
            mockSetup: func(mockDao *mocks.DinosaurDao) {
                mockDao.On("Create", mock.Anything, mock.AnythingOfType("*api.Dinosaur")).
                    Return(nil, errors.GeneralError("Database connection failed"))
            },
            expectedError: "Database connection failed",
        },
        {
            name:  "duplicate species conflict",
            input: factories.BuildDinosaur(factories.DinosaurConfig{Species: "T-Rex"}),
            mockSetup: func(mockDao *mocks.DinosaurDao) {
                mockDao.On("Create", mock.Anything, mock.AnythingOfType("*api.Dinosaur")).
                    Return(nil, errors.Conflict("Dinosaur species already exists"))
            },
            expectedError: "Dinosaur species already exists",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup
            mockDao := &mocks.DinosaurDao{}
            tt.mockSetup(mockDao)
            
            service := NewDinosaurService(DinosaurServiceConfig{
                DinosaurDao: mockDao,
            })
            
            // Execute
            ctx := context.Background()
            result, err := service.Create(ctx, tt.input)
            
            // Assert
            if tt.expectedError != "" {
                assert.Error(t, err)
                assert.Contains(t, err.Error(), tt.expectedError)
                assert.Nil(t, result)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.expectedResult.Species, result.Species)
                assert.NotEmpty(t, result.ID)
                assert.False(t, result.CreatedAt.IsZero())
            }
            
            // Verify mock expectations
            mockDao.AssertExpectations(t)
        })
    }
}

func TestDinosaurService_Update(t *testing.T) {
    t.Run("successful update", func(t *testing.T) {
        // Given
        existing := factories.BuildDinosaur(factories.DinosaurConfig{
            ID:      "existing-id",
            Species: "T-Rex",
            Weight:  &[]float64{7000.0}[0],
        })
        
        updateRequest := &api.DinosaurPatchRequest{
            Weight: &[]float64{8000.0}[0],
        }
        
        mockDao := &mocks.DinosaurDao{}
        mockDao.On("Get", mock.Anything, "existing-id").Return(existing, nil)
        mockDao.On("Update", mock.Anything, mock.AnythingOfType("*api.Dinosaur")).
            Return(func(ctx context.Context, dino *api.Dinosaur) *api.Dinosaur {
                dino.Weight = updateRequest.Weight
                dino.UpdatedAt = time.Now()
                return dino
            }, nil)
        
        service := NewDinosaurService(DinosaurServiceConfig{DinosaurDao: mockDao})
        
        // When
        result, err := service.Update(context.Background(), "existing-id", updateRequest)
        
        // Then
        require.NoError(t, err)
        assert.Equal(t, float64(8000.0), *result.Weight)
        assert.Equal(t, "T-Rex", result.Species) // Unchanged field
        mockDao.AssertExpectations(t)
    })
}

// Benchmark tests for performance-critical functions
func BenchmarkDinosaurService_List(b *testing.B) {
    mockDao := &mocks.DinosaurDao{}
    dinosaurs := make([]api.Dinosaur, 1000)
    for i := range dinosaurs {
        dinosaurs[i] = *factories.BuildDinosaur(factories.DinosaurConfig{})
    }
    
    mockDao.On("List", mock.Anything, mock.Anything).Return(&api.DinosaurList{
        Items: dinosaurs,
        Total: 1000,
    }, nil)
    
    service := NewDinosaurService(DinosaurServiceConfig{DinosaurDao: mockDao})
    ctx := context.Background()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := service.List(ctx, &api.ListOptions{Page: 1, Size: 100})
        if err != nil {
            b.Fatalf("Unexpected error: %v", err)
        }
    }
}
```

### Integration Test Patterns
```go
// File: test/integration/dinosaurs_test.go
package integration

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "testing"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    
    "github.com/openshift-online/rh-trex-ai/pkg/api"
    "github.com/openshift-online/rh-trex-ai/test/factories"
)

func TestDinosaurAPI_CreateDinosaur(t *testing.T) {
    // Setup test environment
    testEnv := NewTestEnvironment(t)
    defer testEnv.Cleanup()
    
    tests := []struct {
        name           string
        payload        interface{}
        authToken      string
        expectedStatus int
        expectedBody   map[string]interface{}
    }{
        {
            name: "successful creation",
            payload: map[string]interface{}{
                "species":    "Triceratops",
                "weight":     6000.0,
                "discovered_date": "2023-01-15T00:00:00Z",
                "location":   "Montana",
            },
            authToken:      testEnv.ValidUserToken(),
            expectedStatus: http.StatusCreated,
            expectedBody: map[string]interface{}{
                "species": "Triceratops",
                "weight":  6000.0,
            },
        },
        {
            name: "validation error - missing species",
            payload: map[string]interface{}{
                "weight": 6000.0,
            },
            authToken:      testEnv.ValidUserToken(),
            expectedStatus: http.StatusBadRequest,
            expectedBody: map[string]interface{}{
                "code":   "TREX-400",
                "reason": "Species is required",
            },
        },
        {
            name:           "unauthorized - missing token",
            payload:        map[string]interface{}{"species": "T-Rex"},
            authToken:      "",
            expectedStatus: http.StatusUnauthorized,
        },
        {
            name:           "unauthorized - invalid token",
            payload:        map[string]interface{}{"species": "T-Rex"},
            authToken:      "invalid-token",
            expectedStatus: http.StatusUnauthorized,
        },
        {
            name: "conflict - duplicate species",
            payload: map[string]interface{}{
                "species": "T-Rex", // Assuming this already exists from setup
            },
            authToken:      testEnv.ValidUserToken(),
            expectedStatus: http.StatusConflict,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Prepare request
            body, err := json.Marshal(tt.payload)
            require.NoError(t, err)
            
            req, err := http.NewRequest("POST", testEnv.BaseURL()+"/api/rh-trex/v1/dinosaurs", bytes.NewReader(body))
            require.NoError(t, err)
            
            req.Header.Set("Content-Type", "application/json")
            if tt.authToken != "" {
                req.Header.Set("Authorization", "Bearer "+tt.authToken)
            }
            
            // Execute request
            resp, err := testEnv.HTTPClient().Do(req)
            require.NoError(t, err)
            defer resp.Body.Close()
            
            // Assert response status
            assert.Equal(t, tt.expectedStatus, resp.StatusCode)
            
            // Assert response body
            if tt.expectedBody != nil {
                var responseBody map[string]interface{}
                err := json.NewDecoder(resp.Body).Decode(&responseBody)
                require.NoError(t, err)
                
                for key, expectedValue := range tt.expectedBody {
                    assert.Equal(t, expectedValue, responseBody[key], 
                        "Mismatch for field %s", key)
                }
            }
            
            // Additional assertions for successful creation
            if tt.expectedStatus == http.StatusCreated {
                var dinosaur api.Dinosaur
                err := json.NewDecoder(resp.Body).Decode(&dinosaur)
                require.NoError(t, err)
                
                assert.NotEmpty(t, dinosaur.ID)
                assert.False(t, dinosaur.CreatedAt.IsZero())
                
                // Verify Location header
                location := resp.Header.Get("Location")
                expectedLocation := fmt.Sprintf("/api/rh-trex/v1/dinosaurs/%s", dinosaur.ID)
                assert.Equal(t, expectedLocation, location)
            }
        })
    }
}

func TestDinosaurAPI_ListDinosaurs(t *testing.T) {
    testEnv := NewTestEnvironment(t)
    defer testEnv.Cleanup()
    
    // Setup test data
    user := testEnv.CreateTestUser()
    dinosaur1 := testEnv.CreateDinosaur(user, factories.DinosaurConfig{Species: "T-Rex"})
    dinosaur2 := testEnv.CreateDinosaur(user, factories.DinosaurConfig{Species: "Triceratops"})
    
    tests := []struct {
        name         string
        queryParams  string
        authToken    string
        expectedIDs  []string
        expectedTotal int
    }{
        {
            name:          "list all dinosaurs",
            authToken:     testEnv.TokenForUser(user),
            expectedIDs:   []string{dinosaur1.ID, dinosaur2.ID},
            expectedTotal: 2,
        },
        {
            name:          "filter by species",
            queryParams:   "?species=T-Rex",
            authToken:     testEnv.TokenForUser(user),
            expectedIDs:   []string{dinosaur1.ID},
            expectedTotal: 1,
        },
        {
            name:          "pagination",
            queryParams:   "?page=1&size=1",
            authToken:     testEnv.TokenForUser(user),
            expectedTotal: 2, // Total should be 2 even with size=1
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            url := testEnv.BaseURL() + "/api/rh-trex/v1/dinosaurs" + tt.queryParams
            req, err := http.NewRequest("GET", url, nil)
            require.NoError(t, err)
            
            if tt.authToken != "" {
                req.Header.Set("Authorization", "Bearer "+tt.authToken)
            }
            
            resp, err := testEnv.HTTPClient().Do(req)
            require.NoError(t, err)
            defer resp.Body.Close()
            
            assert.Equal(t, http.StatusOK, resp.StatusCode)
            
            var list api.DinosaurList
            err = json.NewDecoder(resp.Body).Decode(&list)
            require.NoError(t, err)
            
            assert.Equal(t, tt.expectedTotal, list.Total)
            
            if tt.expectedIDs != nil {
                actualIDs := make([]string, len(list.Items))
                for i, item := range list.Items {
                    actualIDs[i] = item.ID
                }
                assert.ElementsMatch(t, tt.expectedIDs, actualIDs)
            }
        })
    }
}
```

### Test Environment Setup
```go
// File: test/integration/test_environment.go
package integration

import (
    "context"
    "fmt"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
    
    "gorm.io/gorm"
    
    "github.com/openshift-online/rh-trex-ai/cmd/trex/environments"
    "github.com/openshift-online/rh-trex-ai/pkg/api"
    "github.com/openshift-online/rh-trex-ai/pkg/config"
    "github.com/openshift-online/rh-trex-ai/test/factories"
)

type TestEnvironment struct {
    t           *testing.T
    server      *httptest.Server
    httpClient  *http.Client
    db          *gorm.DB
    env         *environments.Env
    cleanup     []func()
}

func NewTestEnvironment(t *testing.T) *TestEnvironment {
    // Setup test database
    testDB := NewTestDB(t)
    
    // Setup test configuration
    config := &config.Config{
        Database: config.DatabaseConfig{
            Debug: true,
        },
        Auth: config.AuthConfig{
            EnableJWT:  false, // Disable JWT for easier testing
            EnableAuthz: false,
        },
    }
    
    // Setup test environment
    env := environments.NewTestEnvironment(config)
    env.SetDatabase(testDB)
    
    // Create test server
    server := httptest.NewServer(env.CreateRouter())
    
    testEnv := &TestEnvironment{
        t:          t,
        server:     server,
        httpClient: &http.Client{Timeout: 10 * time.Second},
        db:         testDB,
        env:        env,
        cleanup:    []func(){},
    }
    
    // Register cleanup
    testEnv.cleanup = append(testEnv.cleanup, func() {
        server.Close()
        testDB.Close()
    })
    
    return testEnv
}

func (e *TestEnvironment) BaseURL() string {
    return e.server.URL
}

func (e *TestEnvironment) HTTPClient() *http.Client {
    return e.httpClient
}

func (e *TestEnvironment) DB() *gorm.DB {
    return e.db
}

func (e *TestEnvironment) Cleanup() {
    for i := len(e.cleanup) - 1; i >= 0; i-- {
        e.cleanup[i]()
    }
}

func (e *TestEnvironment) CreateTestUser() *api.User {
    user := factories.BuildUser(factories.UserConfig{})
    
    // Store user in database for tests that need it
    err := e.db.Create(user).Error
    if err != nil {
        e.t.Fatalf("Failed to create test user: %v", err)
    }
    
    return user
}

func (e *TestEnvironment) CreateDinosaur(user *api.User, config factories.DinosaurConfig) *api.Dinosaur {
    config.CreatedBy = user.ID
    dinosaur := factories.BuildDinosaur(config)
    
    err := e.db.Create(dinosaur).Error
    if err != nil {
        e.t.Fatalf("Failed to create test dinosaur: %v", err)
    }
    
    return dinosaur
}

func (e *TestEnvironment) ValidUserToken() string {
    // For integration tests, return a mock token
    // In real tests, this would generate a valid JWT
    return "valid-test-token-123"
}

func (e *TestEnvironment) TokenForUser(user *api.User) string {
    // Generate token specific to user
    return fmt.Sprintf("user-%s-token", user.ID)
}

func (e *TestEnvironment) AdminToken() string {
    return "admin-test-token-456"
}

// Clear all test data between tests
func (e *TestEnvironment) CleanDB() {
    tables := []string{"fossils", "dinosaurs", "users"}
    for _, table := range tables {
        e.db.Exec(fmt.Sprintf("DELETE FROM %s", table))
    }
}
```

### Test Data Factories
```go
// File: test/factories/dinosaurs.go
package factories

import (
    "time"
    
    "github.com/google/uuid"
    
    "github.com/openshift-online/rh-trex-ai/pkg/api"
)

type DinosaurConfig struct {
    ID             string
    Species        string
    Weight         *float64
    Height         *float64
    Length         *float64
    Era            string
    Diet           string
    Location       string
    DiscoveredDate *time.Time
    CreatedBy      string
    Status         string
}

func BuildDinosaur(config DinosaurConfig) *api.Dinosaur {
    // Set defaults
    if config.ID == "" {
        config.ID = uuid.New().String()
    }
    if config.Species == "" {
        config.Species = "Tyrannosaurus Rex"
    }
    if config.Era == "" {
        config.Era = "Cretaceous"
    }
    if config.Diet == "" {
        config.Diet = "Carnivore"
    }
    if config.Location == "" {
        config.Location = "Montana, USA"
    }
    if config.DiscoveredDate == nil {
        date := time.Date(2023, 1, 15, 0, 0, 0, 0, time.UTC)
        config.DiscoveredDate = &date
    }
    if config.CreatedBy == "" {
        config.CreatedBy = "test-user-id"
    }
    if config.Status == "" {
        config.Status = "active"
    }
    
    dinosaur := &api.Dinosaur{
        Meta: api.Meta{
            ID:        config.ID,
            CreatedAt: time.Now(),
            UpdatedAt: time.Now(),
        },
        Species:        config.Species,
        Weight:         config.Weight,
        Height:         config.Height,
        Length:         config.Length,
        Era:            config.Era,
        Diet:           config.Diet,
        Location:       config.Location,
        DiscoveredDate: config.DiscoveredDate,
        CreatedBy:      config.CreatedBy,
        Status:         config.Status,
    }
    
    return dinosaur
}

// Convenience builders for common scenarios
func BuildTRex() *api.Dinosaur {
    weight := 7000.0
    height := 4.0
    length := 13.0
    
    return BuildDinosaur(DinosaurConfig{
        Species: "Tyrannosaurus Rex",
        Weight:  &weight,
        Height:  &height,
        Length:  &length,
        Era:     "Cretaceous",
        Diet:    "Carnivore",
    })
}

func BuildTriceratops() *api.Dinosaur {
    weight := 6000.0
    height := 3.0
    length := 9.0
    
    return BuildDinosaur(DinosaurConfig{
        Species: "Triceratops",
        Weight:  &weight,
        Height:  &height,
        Length:  &length,
        Era:     "Cretaceous",
        Diet:    "Herbivore",
    })
}

func BuildMinimalDinosaur() *api.Dinosaur {
    return BuildDinosaur(DinosaurConfig{
        Species: "Test Species",
    })
}

// Pointer helper functions for optional fields
func Float64Ptr(f float64) *float64 {
    return &f
}

func TimePtr(t time.Time) *time.Time {
    return &t
}

func StringPtr(s string) *string {
    return &s
}
```

### Mock Generation and Usage
```go
// File: pkg/dao/mocks/dinosaur_dao.go (generated with mockery)
//go:generate mockery --name DinosaurDao --output ../mocks --outpkg mocks

// File: test/mocks/generate.go
package mocks

//go:generate go run github.com/vektra/mockery/v2@latest --all --output . --case underscore
```

### gRPC Testing
```go
// File: test/integration/grpc_test.go
package integration

import (
    "context"
    "testing"
    "time"
    
    "google.golang.org/grpc"
    "google.golang.org/grpc/metadata"
    "google.golang.org/grpc/status"
    "google.golang.org/grpc/codes"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    
    rhtrexv1 "github.com/openshift-online/rh-trex-ai/pkg/api/grpc"
    "github.com/openshift-online/rh-trex-ai/test/factories"
)

func TestGRPCDinosaurService_CreateDinosaur(t *testing.T) {
    testEnv := NewGRPCTestEnvironment(t)
    defer testEnv.Cleanup()
    
    client := rhtrexv1.NewDinosaurServiceClient(testEnv.GRPCConnection())
    
    tests := []struct {
        name           string
        request        *rhtrexv1.CreateDinosaurRequest
        authToken      string
        expectedError  codes.Code
        validateResult func(*rhtrexv1.Dinosaur)
    }{
        {
            name: "successful creation",
            request: &rhtrexv1.CreateDinosaurRequest{
                Species:  "Velociraptor",
                Weight:   15.0,
                Era:      "Cretaceous",
                Diet:     "Carnivore",
                Location: "Mongolia",
            },
            authToken: testEnv.ValidUserToken(),
            validateResult: func(dinosaur *rhtrexv1.Dinosaur) {
                assert.Equal(t, "Velociraptor", dinosaur.Species)
                assert.Equal(t, float32(15.0), dinosaur.Weight)
                assert.NotEmpty(t, dinosaur.Id)
                assert.NotNil(t, dinosaur.CreatedAt)
            },
        },
        {
            name: "validation error",
            request: &rhtrexv1.CreateDinosaurRequest{
                Species: "", // Empty species should fail
            },
            authToken:     testEnv.ValidUserToken(),
            expectedError: codes.InvalidArgument,
        },
        {
            name: "unauthorized",
            request: &rhtrexv1.CreateDinosaurRequest{
                Species: "T-Rex",
            },
            authToken:     "",
            expectedError: codes.Unauthenticated,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup context with auth metadata
            ctx := context.Background()
            if tt.authToken != "" {
                md := metadata.Pairs("authorization", "Bearer "+tt.authToken)
                ctx = metadata.NewOutgoingContext(ctx, md)
            }
            
            // Add timeout
            ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
            defer cancel()
            
            // Execute request
            result, err := client.CreateDinosaur(ctx, tt.request)
            
            // Assert results
            if tt.expectedError != codes.OK {
                require.Error(t, err)
                st, ok := status.FromError(err)
                require.True(t, ok)
                assert.Equal(t, tt.expectedError, st.Code())
            } else {
                require.NoError(t, err)
                require.NotNil(t, result)
                if tt.validateResult != nil {
                    tt.validateResult(result)
                }
            }
        })
    }
}

func TestGRPCDinosaurService_WatchDinosaurs(t *testing.T) {
    testEnv := NewGRPCTestEnvironment(t)
    defer testEnv.Cleanup()
    
    client := rhtrexv1.NewDinosaurServiceClient(testEnv.GRPCConnection())
    
    // Setup auth context
    ctx := context.Background()
    md := metadata.Pairs("authorization", "Bearer "+testEnv.ValidUserToken())
    ctx = metadata.NewOutgoingContext(ctx, md)
    
    // Start watching
    stream, err := client.WatchDinosaurs(ctx, &rhtrexv1.WatchDinosaursRequest{})
    require.NoError(t, err)
    
    // Create a dinosaur in another goroutine to trigger event
    go func() {
        time.Sleep(100 * time.Millisecond)
        testEnv.CreateDinosaur(testEnv.CreateTestUser(), factories.DinosaurConfig{
            Species: "Test Watcher Species",
        })
    }()
    
    // Wait for stream event
    received, err := stream.Recv()
    require.NoError(t, err)
    assert.Equal(t, "Test Watcher Species", received.Species)
    
    stream.CloseSend()
}
```

### Performance and Load Testing
```go
// File: test/performance/load_test.go
package performance

import (
    "context"
    "sync"
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    
    "github.com/openshift-online/rh-trex-ai/test/integration"
    "github.com/openshift-online/rh-trex-ai/test/factories"
)

func TestDinosaurAPI_ConcurrentCreation(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping load test in short mode")
    }
    
    testEnv := integration.NewTestEnvironment(t)
    defer testEnv.Cleanup()
    
    const (
        numGoroutines = 50
        requestsPerGoroutine = 10
    )
    
    var wg sync.WaitGroup
    errors := make(chan error, numGoroutines*requestsPerGoroutine)
    successes := make(chan string, numGoroutines*requestsPerGoroutine)
    
    user := testEnv.CreateTestUser()
    token := testEnv.TokenForUser(user)
    
    startTime := time.Now()
    
    // Launch concurrent goroutines
    for i := 0; i < numGoroutines; i++ {
        wg.Add(1)
        go func(goroutineID int) {
            defer wg.Done()
            
            for j := 0; j < requestsPerGoroutine; j++ {
                dinosaur := factories.BuildDinosaur(factories.DinosaurConfig{
                    Species: fmt.Sprintf("Load-Test-Species-%d-%d", goroutineID, j),
                })
                
                created, err := testEnv.CreateDinosaurViaAPI(token, dinosaur)
                if err != nil {
                    errors <- err
                } else {
                    successes <- created.ID
                }
            }
        }(i)
    }
    
    // Wait for completion
    wg.Wait()
    close(errors)
    close(successes)
    
    duration := time.Since(startTime)
    
    // Collect results
    var errorCount int
    var successCount int
    
    for err := range errors {
        t.Logf("Error: %v", err)
        errorCount++
    }
    
    for range successes {
        successCount++
    }
    
    // Assertions
    totalRequests := numGoroutines * requestsPerGoroutine
    assert.Equal(t, totalRequests, successCount+errorCount)
    
    successRate := float64(successCount) / float64(totalRequests) * 100
    assert.GreaterOrEqual(t, successRate, 95.0, "Success rate should be at least 95%")
    
    throughput := float64(successCount) / duration.Seconds()
    t.Logf("Load test results:")
    t.Logf("  Total requests: %d", totalRequests)
    t.Logf("  Successful: %d", successCount)
    t.Logf("  Failed: %d", errorCount)
    t.Logf("  Success rate: %.2f%%", successRate)
    t.Logf("  Duration: %v", duration)
    t.Logf("  Throughput: %.2f requests/second", throughput)
    
    // Performance assertions
    assert.Less(t, duration, 30*time.Second, "Load test should complete within 30 seconds")
    assert.GreaterOrEqual(t, throughput, 10.0, "Should handle at least 10 requests per second")
}
```

### Test Database Utilities
```go
// File: test/helpers/database.go
package helpers

import (
    "fmt"
    "testing"
    
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
    "gorm.io/gorm/logger"
    
    "github.com/openshift-online/rh-trex-ai/pkg/db/migrations"
)

func NewTestDB(t *testing.T) *gorm.DB {
    // Use environment variable or default to test database
    dbName := fmt.Sprintf("trex_test_%d", time.Now().UnixNano())
    
    dsn := fmt.Sprintf("host=localhost user=test password=test dbname=%s port=5432 sslmode=disable", dbName)
    
    // Create test database
    createDB(dbName)
    
    // Connect to test database
    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
        Logger: logger.Default.LogMode(logger.Silent),
    })
    if err != nil {
        t.Fatalf("Failed to connect to test database: %v", err)
    }
    
    // Run migrations
    if err := runTestMigrations(db); err != nil {
        t.Fatalf("Failed to run test migrations: %v", err)
    }
    
    // Register cleanup
    t.Cleanup(func() {
        dropDB(dbName)
    })
    
    return db
}

func createDB(dbName string) error {
    dsn := "host=localhost user=test password=test dbname=postgres port=5432 sslmode=disable"
    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
    if err != nil {
        return err
    }
    
    return db.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName)).Error
}

func dropDB(dbName string) error {
    dsn := "host=localhost user=test password=test dbname=postgres port=5432 sslmode=disable"
    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
    if err != nil {
        return err
    }
    
    return db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName)).Error
}

func runTestMigrations(db *gorm.DB) error {
    return migrations.Migrate(db)
}
```
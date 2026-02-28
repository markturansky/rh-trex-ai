---
description: Perform comprehensive code review using TRex-specific standards and Red Hat patterns.
---

## Goal
Review code changes against TRex framework standards, Red Hat security requirements, and API best practices.

## Memory System
Load these files to establish review context:

1. `CLAUDE.md` (project overview and commands)
2. `.claude/context/backend-development.md` (Go/TRex patterns)
3. `.claude/context/database-development.md` (PostgreSQL/migration patterns)
4. `.claude/context/security-standards.md` (OCM auth, validation, secrets)
5. `.claude/patterns/error-handling.md` (HTTP status, logging patterns)
6. `.claude/patterns/auth-middleware.md` (OCM integration patterns)
7. `.claude/patterns/testing-patterns.md` (unit/integration standards)

## Review Axes

### 1. **TRex Framework Compliance**
- [ ] Follows generated code patterns (dao/handler/service layers)
- [ ] Uses TRex error handling (`errors.GeneralError`, `errors.BadRequest`)
- [ ] Database operations use GORM properly with transactions
- [ ] OpenAPI spec matches handler implementation
- [ ] gRPC service follows protobuf patterns
- [ ] Plugin registration uses auto-discovery pattern
- [ ] Service locators properly inject dependencies

### 2. **Security (Red Hat Standards)**
- [ ] OCM token validation in auth middleware
- [ ] Input validation prevents injection attacks
- [ ] No secrets in logs (use `len(token)`, redact sensitive data)
- [ ] RBAC checks before resource access
- [ ] Database queries use parameterized statements
- [ ] Error messages don't leak internal details
- [ ] File uploads validated for type and size

### 3. **Database & Migrations**
- [ ] Migrations are reversible with Down() method
- [ ] Database schema follows TRex conventions
- [ ] GORM models use proper tags and relationships
- [ ] Transactions used for multi-table operations
- [ ] Advisory locks prevent race conditions
- [ ] Migration registration in migration_structs.go
- [ ] No raw SQL without parameterization

### 4. **Testing Standards**
- [ ] Unit tests cover new business logic
- [ ] Integration tests validate API endpoints
- [ ] Test factories provide consistent test data
- [ ] TestMain sets up/tears down test database
- [ ] Tests use testcontainers for isolation
- [ ] Event-driven controllers have test coverage
- [ ] gRPC streaming tests validate message flow

### 5. **Performance & Reliability**
- [ ] Database queries avoid N+1 problems
- [ ] Pagination implemented for list endpoints
- [ ] Proper HTTP status codes (404, 409, 422, 500)
- [ ] Graceful error handling doesn't crash service
- [ ] Resource cleanup in defer statements
- [ ] Context timeout handling in long operations
- [ ] Memory efficient streaming for large responses

### 6. **API Design**
- [ ] REST endpoints follow TRex naming conventions
- [ ] OpenAPI spec includes proper examples
- [ ] Request/response models use proper validation tags
- [ ] Patch operations support partial updates
- [ ] List operations support filtering and sorting
- [ ] gRPC services implement health checks
- [ ] Backward compatibility maintained in API changes

### 7. **Event-Driven Architecture**
- [ ] Controllers implement idempotent handlers
- [ ] Event handlers use structured logging
- [ ] PostgreSQL LISTEN/NOTIFY used correctly
- [ ] Event processing doesn't block main goroutine
- [ ] Error handling in event callbacks
- [ ] Event broker manages subscriber cleanup

## Severity Levels
- **Blocker**: Security vulnerabilities, data corruption risk, server crashes
- **Critical**: Framework violations, missing auth, panic() calls, breaking changes
- **Major**: Poor error handling, missing tests, performance issues, API design flaws
- **Minor**: Style, naming, documentation gaps, non-essential optimizations

## Review Process

### 1. Pre-Review Setup
```bash
# Load all context files
cat CLAUDE.md
cat .claude/context/backend-development.md
cat .claude/context/database-development.md  
cat .claude/context/security-standards.md
cat .claude/patterns/error-handling.md
cat .claude/patterns/auth-middleware.md
cat .claude/patterns/testing-patterns.md
```

### 2. Code Analysis
- Review all modified files against each axis
- Check for framework pattern compliance
- Validate security implementations
- Verify test coverage and quality

### 3. Report Format
```markdown
## TRex Code Review Summary

### Overview
- **Files Reviewed**: N files
- **Severity Distribution**: X Blocker, Y Critical, Z Major, W Minor

### Critical Issues
1. **[BLOCKER]** Description of security/data issue
2. **[CRITICAL]** Description of framework violation

### Recommendations
1. **Security**: Specific auth/validation improvements
2. **Framework**: TRex pattern compliance fixes
3. **Testing**: Coverage gaps to address

### Approval Status
- [ ] **Approved** - Ready for merge
- [ ] **Approved with Minor Changes** - Address minor issues post-merge
- [ ] **Changes Requested** - Must fix critical/major issues before merge
- [ ] **Rejected** - Significant rework required
```

### 4. Follow-up Actions
- Provide specific code examples for fixes
- Link to relevant TRex documentation
- Suggest refactoring opportunities
- Recommend additional testing scenarios
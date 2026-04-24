# Monolith Architecture Example

This example demonstrates a traditional 3-tier monolithic application using the C4 Model.

## Architecture Overview

**Pattern**: Layered monolith (3-tier)
**Domain**: Content Management System (CMS)
**Scale**: Small to medium (single application server)
**Technologies**: Java Spring Boot, PostgreSQL, Redis

## C4 Diagrams

### Level 1: Context Diagram
**File**: `c4-context.d2`
**Shows**: System boundary, users, and external systems

**Key Elements**:
- Content Editor (Person)
- Website Visitor (Person)
- CMS Platform (Software System - our monolith)
- CDN (External System)
- Analytics Service (External System)

### Level 2: Container Diagram
**File**: `c4-container.d2`
**Shows**: Application tiers and databases

**Containers**:
- Web Application (Spring Boot monolith)
- PostgreSQL Database
- Redis Cache
- Static File Storage
- Admin Dashboard (React)

## Rendering

```bash
d2 c4-context.d2 rendered/c4-context.png
d2 c4-container.d2 rendered/c4-container.png
```

## Key Design Patterns

1. **Layered Architecture**: Presentation → Business Logic → Data Access
2. **MVC Pattern**: Model-View-Controller separation
3. **Repository Pattern**: Data access abstraction
4. **Service Layer**: Business logic encapsulation
5. **Caching**: Redis for frequently accessed content

## Architecture Decisions

**Why monolith?**
- Simple deployment (single JAR/WAR)
- Easier development (no distributed complexity)
- Good for small-medium traffic
- Lower operational overhead

**Why Spring Boot?**
- Mature ecosystem
- Convention over configuration
- Good performance
- Easy to maintain

**Why PostgreSQL?**
- ACID compliance
- Rich feature set
- Good for content storage
- Full-text search capabilities

## Scaling Strategy

**Vertical Scaling**:
- Add more CPU/RAM to single server
- Increase database connection pool
- Optimize query performance

**Horizontal Scaling** (when needed):
- Load balancer + multiple application instances
- Shared database (single source of truth)
- Redis for session sharing
- CDN for static assets

## Migration Path to Microservices

If/when monolith becomes too large:

1. **Extract Content Delivery** → Separate service
2. **Extract User Management** → Separate service
3. **Extract Media Processing** → Separate service
4. **Keep CMS Core** → Simplified monolith

## Related Files

- `c4-context.d2` - System context diagram
- `c4-container.d2` - Container/tier diagram

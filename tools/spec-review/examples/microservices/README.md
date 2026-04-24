# Microservices Architecture Example

This example demonstrates a modern microservices e-commerce platform using the C4 Model.

## Architecture Overview

**Pattern**: Microservices with API Gateway
**Domain**: E-commerce platform
**Scale**: Medium (10-50 services)
**Technologies**: Go, Node.js, Python, PostgreSQL, Redis, RabbitMQ

## C4 Diagrams

### Level 1: Context Diagram
**File**: `c4-context.d2`
**Shows**: System boundary, external users, and external systems

**Key Elements**:
- Customer (Person)
- E-Commerce Platform (Software System - our system)
- Payment Gateway (External System)
- Email Service (External System)
- Inventory System (External System)

### Level 2: Container Diagram
**File**: `c4-container.d2`
**Shows**: Major containers (services, databases, message queues)

**Containers**:
- API Gateway (Go)
- User Service (Node.js)
- Product Service (Go)
- Order Service (Python)
- Payment Service (Go)
- Notification Service (Node.js)
- Web App (React)
- Mobile App (React Native)
- PostgreSQL databases (per service)
- Redis cache
- RabbitMQ message queue

### Level 3: Component Diagram (Order Service)
**File**: `c4-component-order-service.d2`
**Shows**: Internal components of Order Service

**Components**:
- Order API (REST endpoints)
- Order Repository (Database access)
- Payment Client (External API client)
- Inventory Client (External API client)
- Event Publisher (Message queue publisher)

## Rendering

### Render all diagrams to PNG:
```bash
d2 c4-context.d2 rendered/c4-context.png
d2 c4-container.d2 rendered/c4-container.png
d2 c4-component-order-service.d2 rendered/c4-component-order-service.png
```

### Render to SVG (scalable):
```bash
d2 c4-context.d2 rendered/c4-context.svg
d2 c4-container.d2 rendered/c4-container.svg
d2 c4-component-order-service.d2 rendered/c4-component-order-service.svg
```

## Key Design Patterns

1. **API Gateway Pattern**: Single entry point for all clients
2. **Database per Service**: Each microservice has its own database
3. **Event-Driven Communication**: RabbitMQ for async communication
4. **CQRS (Simplified)**: Separate read/write paths in Order Service
5. **Circuit Breaker**: Payment Service has fallback mechanisms

## Architecture Decisions

**Why microservices?**
- Independent deployment and scaling
- Technology diversity (Go, Node.js, Python)
- Team autonomy (domain-driven ownership)

**Why RabbitMQ?**
- Reliable message delivery
- Mature ecosystem
- Good performance for medium scale

**Why PostgreSQL?**
- Strong consistency guarantees
- Rich query capabilities
- Good ecosystem support

## Scaling Characteristics

**User Service**: High read, low write (caching effective)
**Product Service**: Read-heavy (Redis cache critical)
**Order Service**: Write-heavy (queue for async processing)
**Payment Service**: Low volume, high reliability (circuit breakers)
**Notification Service**: Async, queue-based (can scale independently)

## Related Files

- `c4-context.d2` - System context diagram
- `c4-container.d2` - Container/service diagram
- `c4-component-order-service.d2` - Order service internals
- `deployment.d2` - Kubernetes deployment architecture (optional)

# Event-Driven Architecture Example

This example demonstrates a CQRS + Event Sourcing architecture using the C4 Model.

## Architecture Overview

**Pattern**: CQRS (Command Query Responsibility Segregation) + Event Sourcing
**Domain**: Banking/Financial platform
**Scale**: Large (high throughput, audit requirements)
**Technologies**: Go, Kafka, PostgreSQL, MongoDB, Redis

## C4 Diagrams

### Level 1: Context Diagram
**File**: `c4-context.mmd`
**Shows**: System boundary, users, and external systems

**Key Elements**:
- Account Holder (Person)
- Banking Platform (Software System)
- Payment Network (External System)
- Regulatory Reporting (External System)

### Level 2: Container Diagram
**File**: `c4-container.d2`
**Shows**: CQRS separation and event store

**Containers**:
- Command API (Write operations)
- Query API (Read operations)
- Event Store (Kafka)
- Write Model (PostgreSQL)
- Read Model (MongoDB)
- Projection Service
- Saga Orchestrator

## Rendering

```bash
# D2 diagrams
d2 c4-container.d2 rendered/c4-container.png

# Mermaid diagram
mmdc -i c4-context.mmd -o rendered/c4-context.png
```

## Key Design Patterns

1. **CQRS**: Separate read and write models
2. **Event Sourcing**: Store all state changes as events
3. **Saga Pattern**: Distributed transaction management
4. **Projection**: Build read models from event stream
5. **Event-Driven**: Loosely coupled services via events

## Architecture Decisions

**Why CQRS?**
- Optimized read/write paths
- Different scaling characteristics
- Complex query requirements
- Read-heavy workload

**Why Event Sourcing?**
- Complete audit trail (regulatory requirement)
- Temporal queries (account state at any point in time)
- Event replay capability
- Business analytics from event stream

**Why Kafka?**
- High throughput event streaming
- Durable message storage
- Event ordering guarantees
- Scalable partitioning

## Event Flow

### Write Path (Commands)
```
1. User submits command → Command API
2. Command API validates business rules
3. Generate events (e.g., AccountDebited)
4. Append events to Event Store (Kafka)
5. Write Model updated (current state in PostgreSQL)
6. Events published to topic
```

### Read Path (Queries)
```
1. User queries data → Query API
2. Query API reads from Read Model (MongoDB)
3. Pre-computed projections returned
4. Fast response (optimized for reads)
```

### Projection Path (Async)
```
1. Projection Service consumes events from Kafka
2. Updates Read Model (MongoDB)
3. Maintains materialized views
4. Denormalized for query performance
```

## Event Types

**Account Events**:
- AccountCreated
- AccountDebited
- AccountCredited
- AccountFrozen
- AccountClosed

**Transaction Events**:
- TransactionInitiated
- TransactionCompleted
- TransactionFailed
- TransactionReversed

**Saga Events**:
- TransferStarted
- FundsReserved
- FundsTransferred
- TransferCompleted
- TransferFailed
- CompensationStarted

## Scaling Characteristics

**Command API**: Low volume, high latency tolerance
**Query API**: High volume, low latency required
**Event Store**: Append-only, horizontally scalable
**Projection Service**: Can lag behind (eventual consistency)

## Trade-offs

**Benefits**:
- ✅ Complete audit trail
- ✅ Temporal queries
- ✅ Optimized read/write paths
- ✅ Event replay capability

**Challenges**:
- ❌ Eventual consistency
- ❌ Complex projections
- ❌ Increased operational complexity
- ❌ Event schema evolution

## Related Files

- `c4-context.mmd` - System context diagram (Mermaid)
- `c4-container.d2` - Container/service diagram (D2)

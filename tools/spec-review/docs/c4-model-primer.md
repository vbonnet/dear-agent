# C4 Model Primer

A comprehensive guide to the C4 Model for software architecture diagramming.

## Table of Contents

1. [What is the C4 Model?](#what-is-the-c4-model)
2. [The Four Levels](#the-four-levels)
3. [Core Concepts](#core-concepts)
4. [When to Use Each Level](#when-to-use-each-level)
5. [Element Types](#element-types)
6. [Relationship Types](#relationship-types)
7. [Best Practices](#best-practices)
8. [Common Mistakes](#common-mistakes)
9. [Tools and Formats](#tools-and-formats)

---

## What is the C4 Model?

The **C4 Model** is a lean graphical notation technique for modeling software architecture. Created by Simon Brown, it provides a hierarchical approach to diagramming with four levels of abstraction:

- **C1**: Context
- **C2**: Container
- **C3**: Component
- **C4**: Code

**Key Principles**:
- **Progressive disclosure**: Start high-level, zoom in as needed
- **Audience-appropriate detail**: Different stakeholders need different views
- **Semantic notation**: Clear element types (Person, System, Container, Component)
- **Abstraction layers**: Prevents mixing levels of detail

**Benefits**:
- ✅ **Consistent notation** across teams
- ✅ **Clear communication** with non-technical stakeholders
- ✅ **Prevents over-detailing** (UML class diagrams for everything)
- ✅ **Scalable** from startup to enterprise

---

## The Four Levels

### Level 1: System Context Diagram

**Purpose**: Show the big picture - your system and its users/dependencies

**Audience**: Everyone (executives, product managers, developers, customers)

**Shows**:
- Your software system (the thing you're building)
- People who use it (actors, users, personas)
- Other systems it interacts with (external dependencies)

**Does NOT show**:
- Internal structure of your system
- Technology choices
- Implementation details

**Example Questions Answered**:
- What does this system do?
- Who uses it?
- What other systems does it depend on?
- What are the boundaries?

**Diagram Size**: 1 diagram per system

---

### Level 2: Container Diagram

**Purpose**: Show the high-level technology choices and how containers communicate

**Audience**: Technical stakeholders (developers, architects, DevOps)

**Shows**:
- **Containers**: Deployable/runnable units (web apps, mobile apps, databases, file systems)
- **Technology stack**: Programming languages, frameworks
- **Communication**: How containers interact (HTTP, gRPC, message queues)

**Does NOT show**:
- Internal code structure of containers
- Classes or modules
- Detailed API contracts

**Container Examples**:
- Web application (React SPA)
- API server (Node.js + Express)
- Database (PostgreSQL)
- Message queue (RabbitMQ)
- Mobile app (iOS/Android)

**Example Questions Answered**:
- What are the major building blocks?
- What technologies are used?
- How do the pieces communicate?
- Where is data stored?

**Diagram Size**: 1 diagram per system, or multiple for large systems

---

### Level 3: Component Diagram

**Purpose**: Show the internal structure of a single container

**Audience**: Developers, architects working on specific containers

**Shows**:
- **Components**: Groupings of related functionality (services, repositories, controllers)
- **Responsibilities**: What each component does
- **Dependencies**: How components depend on each other

**Does NOT show**:
- Individual classes (that's Level 4)
- Implementation details
- Every function or method

**Component Examples**:
- OrderController (handles order HTTP requests)
- OrderService (business logic for orders)
- OrderRepository (database access for orders)
- PaymentClient (integrates with payment gateway)

**Example Questions Answered**:
- How is this container organized internally?
- What are the main components?
- How do they interact?
- What are the key abstractions?

**Diagram Size**: 1 diagram per container (only for complex containers)

---

### Level 4: Code Diagram

**Purpose**: Show actual code structure (classes, interfaces)

**Audience**: Developers implementing the code

**Reality**: **Most teams skip this level** because:
- UML class diagrams are verbose and hard to maintain
- IDEs can generate class diagrams automatically
- Code is the source of truth (diagrams get stale)

**When to use Level 4**:
- Complex design patterns (Strategy, Factory, Observer)
- Teaching/onboarding scenarios
- Critical algorithms with complex structure

**Alternative**: Use code comments and docstrings instead of diagrams

---

## Core Concepts

### Element Types

| Type | Description | Used In |
|------|-------------|---------|
| **Person** | Human user or actor | Level 1 |
| **Software System** | The highest level system boundary | Level 1 |
| **External System** | Systems you depend on but don't control | Level 1 |
| **Container** | Deployable/runnable unit (app, database, etc.) | Level 2 |
| **Component** | Grouping of related code | Level 3 |
| **Code Element** | Class, interface, function | Level 4 |

### Relationship Types

| Type | Description | Example |
|------|-------------|---------|
| **Uses** | General dependency | Customer uses E-Commerce System |
| **Reads from** | Data read operation | API reads from Database |
| **Writes to** | Data write operation | API writes to Database |
| **Sends to** | Message/event sending | Order Service sends to Queue |
| **Includes** | Composition | Container includes Components |

---

## When to Use Each Level

### Decision Matrix

| Project Size | Context | Container | Component | Code |
|--------------|---------|-----------|-----------|------|
| **Small** (1-3 services) | ✅ Always | ✅ Recommended | ❌ Skip | ❌ Skip |
| **Medium** (4-20 services) | ✅ Always | ✅ Always | ✅ For complex containers | ❌ Skip |
| **Large** (20+ services) | ✅ Always | ✅ Always | ✅ For critical containers | 🟡 Rare |

### Progressive Rigor

Start simple, add detail only when needed:

1. **Week 1**: Context diagram (understand the problem space)
2. **Week 2**: Container diagram (plan the solution)
3. **Sprint 1-2**: Component diagrams for core containers
4. **As needed**: Code diagrams for complex components

### Wayfinder Integration

| Wayfinder Phase | C4 Level | Purpose |
|-----------------|----------|---------|
| **D4 (Requirements)** | Context | Define system boundary |
| **S6 (Design)** | Container + Component | Architecture design |
| **S8 (Implementation)** | Sync check | Keep diagrams current |
| **S11 (Retrospective)** | Final validation | Quality and sync status |

---

## Element Types in Detail

### Level 1: Context Elements

#### Person
**What**: Human user or actor
**Examples**:
- Customer
- Administrator
- Support Agent
- System Operator

**Diagram Notation**:
- Shape: Stick figure or person icon
- Color: Blue (#08427b)
- Label: Role name (not individual names)

#### Software System
**What**: The system you're building (or a peer system)
**Examples**:
- E-Commerce Platform
- Mobile Banking App
- Inventory Management System

**Diagram Notation**:
- Shape: Rectangle
- Color: Blue (#1168bd) for your system
- Label: System name + brief description

#### External System
**What**: Systems you depend on but don't own
**Examples**:
- Payment Gateway (Stripe)
- Email Service (SendGrid)
- Legacy CRM System

**Diagram Notation**:
- Shape: Rectangle
- Color: Gray (#999999)
- Label: System name + technology/vendor

---

### Level 2: Container Elements

#### Container Types

**Web Application**:
- React, Angular, Vue.js SPAs
- Server-rendered apps (Next.js, Rails)
- Static sites

**API/Service**:
- REST APIs (Node.js, Go, Python)
- GraphQL APIs
- gRPC services

**Database**:
- Relational (PostgreSQL, MySQL)
- NoSQL (MongoDB, Redis)
- Search (Elasticsearch)

**Message Queue**:
- RabbitMQ, Kafka, SQS
- Event streams
- Pub/sub systems

**Mobile App**:
- iOS (Swift)
- Android (Kotlin)
- Cross-platform (React Native, Flutter)

**File System**:
- S3, NFS
- Document stores
- Media storage

**Diagram Notation**:
- Shape: Rectangle (apps), Cylinder (databases), Queue (message brokers)
- Color: Light blue (#438dd5)
- Label: Name + technology stack

---

### Level 3: Component Elements

#### Component Types

**Controllers/Handlers**:
- HTTP request handlers
- Event handlers
- Command handlers

**Services**:
- Business logic
- Domain services
- Application services

**Repositories**:
- Data access layer
- Database abstraction
- ORM wrappers

**Clients**:
- External API clients
- HTTP clients
- gRPC clients

**Utilities**:
- Logging
- Authentication
- Validation

**Diagram Notation**:
- Shape: Rectangle
- Color: Blue (#1168bd)
- Label: Component name + responsibility

---

## Relationship Types in Detail

### Level 1 Relationships

**Person → System**:
- "Uses"
- "Manages"
- "Administers"

**System → System**:
- "Uses"
- "Sends data to"
- "Receives data from"
- "Authenticates via"

---

### Level 2 Relationships

**Container → Container**:
- "Makes API calls to" (HTTP/gRPC)
- "Reads from / Writes to" (Database)
- "Publishes to / Consumes from" (Message Queue)

**Technology-Specific**:
- REST API: `POST /orders (HTTPS/JSON)`
- gRPC: `CreateOrder (gRPC/Protobuf)`
- Database: `SELECT/INSERT (SQL)`
- Queue: `Publishes OrderCreated event`

---

### Level 3 Relationships

**Component → Component**:
- "Uses"
- "Depends on"
- "Calls"
- "Delegates to"

**Component → External**:
- "Reads from Database"
- "Calls External API"
- "Publishes events to Queue"

---

## Best Practices

### Naming Conventions

**Elements**:
- ✅ Good: "E-Commerce Platform", "Order Service", "User Repository"
- ❌ Bad: "System1", "Service", "Database"

**Relationships**:
- ✅ Good: "Reads customer data via REST API (HTTPS/JSON)"
- ❌ Bad: "Uses", "Calls" (too vague)

### Level Separation

**Don't Mix Levels**:
- ❌ Bad: Context diagram showing database tables
- ❌ Bad: Container diagram showing individual classes
- ✅ Good: Each diagram at consistent abstraction level

### Diagram Size

**Keep it manageable**:
- Context: 3-10 elements
- Container: 5-20 elements
- Component: 5-15 components per container

**If too large**:
- Split into multiple diagrams
- Create filtered views
- Focus on specific areas

### Technology Details

**Include where helpful**:
- ✅ "PostgreSQL 14" (database version matters)
- ✅ "Node.js + Express" (framework choice is relevant)
- ❌ "Written in Go" on Context diagram (too detailed)

### Labels and Descriptions

**Every element should have**:
- **Name**: Clear, descriptive
- **Type**: Implicit from shape/color
- **Technology** (Level 2+): Tech stack
- **Purpose** (optional): Brief description

### Colors and Styling

**Standard C4 Palette**:
- Person: Blue (#08427b)
- Your System: Blue (#1168bd)
- External System: Gray (#999999)
- Container: Light blue (#438dd5)
- Database: Blue (#438dd5) with cylinder shape

**Custom Colors** (use sparingly):
- Red: Deprecated/legacy systems
- Orange: Systems being replaced
- Green: New/future systems

---

## Common Mistakes

### Mistake 1: Skipping Context Diagrams

**Problem**: Jumping straight to Container diagrams
**Impact**: Stakeholders don't understand the big picture
**Fix**: Always start with Context, even if it seems simple

### Mistake 2: Too Much Detail Too Soon

**Problem**: Adding implementation details to high-level diagrams
**Impact**: Diagrams become cluttered and hard to read
**Fix**: Follow progressive disclosure - start high, zoom in later

### Mistake 3: Outdated Diagrams

**Problem**: Diagrams created once and never updated
**Impact**: Diagrams mislead rather than help
**Fix**: Use diagram-sync tool, integrate into CI/CD, enforce in reviews

### Mistake 4: UML Overuse

**Problem**: Treating C4 like UML with full notation
**Impact**: Diagrams become too complex
**Fix**: C4 is deliberately simple - keep it that way

### Mistake 5: One Diagram to Rule Them All

**Problem**: Trying to show everything in one diagram
**Impact**: Information overload
**Fix**: Create multiple focused diagrams

### Mistake 6: No Legend

**Problem**: Viewers don't know what shapes/colors mean
**Impact**: Misinterpretation
**Fix**: Always include a legend

### Mistake 7: Inconsistent Notation

**Problem**: Using different colors/shapes for same element types
**Impact**: Confusion
**Fix**: Establish and follow notation conventions

---

## Tools and Formats

### Recommended Tools

**D2** (Recommended):
- Modern declarative syntax
- Multiple layout engines
- Native Go library
- Great for code generation

**Structurizr DSL**:
- C4 Model native format
- Model/views separation
- Strong C4 compliance
- Good for large architectures

**Mermaid**:
- Embedded in Markdown
- GitHub rendering support
- C4 diagram support improving
- Good for documentation

**PlantUML** (Legacy):
- Widely used but dated
- Complex syntax
- Migration recommended

### Diagram-as-Code Benefits

**Version Control**:
- Diagrams in Git alongside code
- Diff and merge support
- Code review integration

**Automation**:
- Generate from codebase
- Auto-update on changes
- CI/CD validation

**Collaboration**:
- Text-based (no binary files)
- Easy to review in PRs
- No vendor lock-in

### Engram Skills Integration

Use engram skills for C4 workflows:

```bash
# Generate C4 diagrams from codebase
create-diagrams /path/to/code diagrams/ --level all

# Validate diagram quality
review-diagrams diagrams/c4-context.d2

# Render to images
render-diagrams diagrams/*.d2 rendered/

# Check diagram-code sync
diagram-sync diagrams/ src/
```

---

## Examples

See full examples in:
- `examples/microservices/` - Microservices architecture
- `examples/monolith/` - Traditional 3-tier monolith
- `examples/event-driven/` - CQRS + Event Sourcing

---

## Further Reading

**Official C4 Model Resources**:
- [c4model.com](https://c4model.com) - Official C4 Model website
- Simon Brown's book: "Software Architecture for Developers"

**Diagram Tools**:
- [D2 Language](https://d2lang.com) - Modern diagram-as-code
- [Structurizr](https://structurizr.com) - C4 Model native tool
- [Mermaid](https://mermaid.js.org) - Markdown-embedded diagrams

**Engram Documentation**:
- `skills/create-diagrams/SKILL.md` - Auto-generation from code
- `skills/review-diagrams/SKILL.md` - Quality validation
- `skills/diagram-sync/SKILL.md` - Drift detection

---

## Quick Reference

### C4 Levels Summary

| Level | Purpose | Audience | Elements |
|-------|---------|----------|----------|
| **1: Context** | System boundaries | Everyone | Person, System |
| **2: Container** | Tech stack | Technical | Container, Database |
| **3: Component** | Internal structure | Developers | Component |
| **4: Code** | Classes | Developers | Class, Interface |

### Decision Tree

```
Start here → Create Context diagram (always)
           ↓
  Do you have 2+ containers? → No  → Stop here
           ↓ Yes
  Create Container diagram
           ↓
  Do you have complex containers? → No  → Stop here
           ↓ Yes
  Create Component diagrams (for complex containers only)
           ↓
  Do you have complex design patterns? → No  → Stop here
           ↓ Yes
  Create Code diagrams (rarely needed)
```

---

**Document Version**: 1.0
**Last Updated**: 2026-03-12
**Maintained by**: Engram Team

# Product Specification: {{project_name}}

**Version:** {{version}}
**Last Updated:** {{last_updated}}
**Status:** {{status}}
**Contributors:** {{contributors}}
**Stakeholders:** {{stakeholders}}

---

## 1. Vision

**What is this:** {{what_is_this}}

**Problem Statement:**

{{problem_statement}}

**Who Benefits:**

{{who_benefits}}

**Product Vision:**

{{product_vision}}

---

## 2. User Personas

{{#personas}}
### Persona {{persona_number}}: {{persona_name}}

**Demographics:** {{demographics}}

**Goals:**
{{#goals}}
- {{goal}}
{{/goals}}

**Pain Points:**
{{#pain_points}}
- {{pain_point}}
{{/pain_points}}

**Behaviors:**
{{#behaviors}}
- {{behavior}}
{{/behaviors}}

**Jobs-to-be-Done:**
{{#jobs_to_be_done}}
- {{job}}
{{/jobs_to_be_done}}

{{/personas}}

---

## 3. Critical User Journeys (CUJs)

{{#cujs}}
### CUJ {{cuj_number}}: {{cuj_name}} ({{cuj_type}})

**Goal:** {{cuj_goal}}

**Lifecycle Stage:** {{lifecycle_stage}}

**Tasks:**

{{#tasks}}
#### Task {{task_number}}: {{task_name}}
- **Intent:** {{intent}}
- **Action:** {{action}}
- **Success Criteria:** {{success_criteria}}

{{/tasks}}

**Metrics:**
{{#metrics}}
- {{metric}}
{{/metrics}}

{{/cujs}}

---

## 4. Goals & Success Metrics

{{#goals_section}}
### Goal {{goal_number}}: {{goal_name}}

**Description:** {{description}}

**North Star Metric:** {{north_star_metric}}

**Success Criteria:**

#### Primary Metrics
{{#primary_metrics}}
- {{metric}}
{{/primary_metrics}}

#### Secondary Metrics
{{#secondary_metrics}}
- {{metric}}
{{/secondary_metrics}}

{{#efficiency_metrics}}
#### Efficiency Metrics
{{#metrics}}
- {{metric}}
{{/metrics}}
{{/efficiency_metrics}}

**How to Measure:**
{{how_to_measure}}

{{/goals_section}}

---

## 5. Feature Prioritization (MoSCoW)

### Must Have
{{#must_have_features}}
**{{feature_id}}: {{feature_name}}**
- Why critical: {{why_critical}}
- Effort: {{effort}}
- Status: {{status}}

{{/must_have_features}}

### Should Have
{{#should_have_features}}
**{{feature_id}}: {{feature_name}}**
- Important: {{importance}}
- Effort: {{effort}}

{{/should_have_features}}

### Could Have
{{#could_have_features}}
**{{feature_id}}: {{feature_name}}**
- Nice-to-have: {{rationale}}
- Effort: {{effort}}

{{/could_have_features}}

### Won't Have (This Release)
{{#wont_have_features}}
**{{feature_id}}: {{feature_name}}**
- Deferred because: {{reason}}

{{/wont_have_features}}

---

## 6. Scope Boundaries

### In Scope

**Functional Features:**
{{#in_scope_functional}}
- {{feature}}
{{/in_scope_functional}}

**Non-Functional Requirements:**
{{#in_scope_nonfunctional}}
- {{requirement}}
{{/in_scope_nonfunctional}}

**Target User Segments:**
{{#target_segments}}
- {{segment}}
{{/target_segments}}

**Supported Platforms:**
{{#supported_platforms}}
- {{platform}}
{{/supported_platforms}}

### Out of Scope (Explicit Exclusions)

**Features Deferred:**
{{#out_of_scope}}
- {{feature}} ({{reason}})
{{/out_of_scope}}

---

## 7. Assumptions & Constraints

### Assumptions
{{#assumptions}}
**Assumption {{assumption_number}}:** {{assumption}}
- Impact: {{impact}}
- Validation: {{validation}}

{{/assumptions}}

### Constraints

**Technical:**
{{#technical_constraints}}
- {{constraint}}
{{/technical_constraints}}

**Organizational:**
{{#organizational_constraints}}
- {{constraint}}
{{/organizational_constraints}}

**Resource:**
{{#resource_constraints}}
- {{constraint}}
{{/resource_constraints}}

---

## 8. Agent-Specific Specifications

### Agent Goals (Declarative)

**Goal:** {{agent_goal}}

**Constraints:**
{{#agent_constraints}}
- {{constraint}}
{{/agent_constraints}}

**Success Criteria:** See Goal 1 in Section 4

**Explicitly Unacceptable:**
{{#unacceptable_behaviors}}
- {{behavior}}
{{/unacceptable_behaviors}}

---

## 9. Living Document Process

### When to Update

{{#update_triggers}}
- {{trigger}}
{{/update_triggers}}

### How to Update

{{#update_steps}}
{{step_number}}. {{step}}
{{/update_steps}}

### Related Documents

{{#related_docs}}
- **{{doc_name}}:** {{doc_description}}
{{/related_docs}}

---

## 10. Version History

| Version | Date | Changes | Rationale |
|---------|------|---------|-----------|
{{#version_history}}
| {{version}} | {{date}} | {{changes}} | {{rationale}} |
{{/version_history}}

---

## Appendix

### A. Quality Rubric

{{quality_rubric}}

### B. Architecture Diagrams

**Location:** {{diagram_location|default:diagrams/}}

**C4 Model Diagrams:**

{{#has_diagrams}}
- **Context Diagram (Level 1):** Shows system boundary and external actors
  - File: `{{context_diagram_path|default:diagrams/c4-context.d2}}`
  - Purpose: High-level view for stakeholders

- **Container Diagram (Level 2):** Shows runtime architecture and deployable units
  - File: `{{container_diagram_path|default:diagrams/c4-container.d2}}`
  - Purpose: Technical architecture overview

- **Component Diagram (Level 3):** Shows internal structure (optional)
  - File: `{{component_diagram_path|default:diagrams/c4-component.d2}}`
  - Purpose: Detailed component design
{{/has_diagrams}}

{{^has_diagrams}}
**Note:** Architecture diagrams not yet created.

To generate skeleton diagrams:
```bash
python skills/create-spec/create_spec.py . --generate-diagrams
```

Or create manually using:
- **D2** (.d2 files) - Recommended, modern diagram-as-code
- **Structurizr DSL** (.dsl files) - C4 model native format
- **Mermaid** (.mmd files) - Widely supported, GitHub-compatible
{{/has_diagrams}}

**Rendering Instructions:**

```bash
# D2 diagrams
d2 diagrams/c4-context.d2 diagrams/c4-context.svg

# Structurizr DSL
structurizr-cli export -workspace diagrams/workspace.dsl -format svg

# Mermaid
mmdc -i diagrams/c4-context.mmd -o diagrams/c4-context.svg
```

**Validation:**

Check diagram-code sync with:
```bash
python skills/diagram-sync/diagram_sync.py diagrams/c4-context.d2 .
```

### C. Technical References

{{#technical_references}}
- **{{ref_name}}:** `{{ref_path}}`
{{/technical_references}}

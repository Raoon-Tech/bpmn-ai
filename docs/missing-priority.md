# Missing BPMN Implementations by Priority

🌐 **English** | [Español](faltantes-prioridad.md)

## 🔴 High Priority

| # | Feature | Impact |
|---|---------|--------|
| 1 | **Authentication/Authorization** with JWT middleware | API security |

## 🟡 Medium Priority

| # | Feature | Impact |
|---|---------|--------|
| 5 | **Business Rule Task** (DMN engine) | Emulable with ScriptTask but no dedicated rule engine |
| 6 | **Send Task / Receive Task** as dedicated elements | Emulable with MessageThrow/Catch but without explicit semantics |
| 7 | **Data Objects / Data Stores** with formal persistence | Flat variables used instead — no formal data semantics |
| 8 | **Pools / Lanes** for participant collaboration | No process collaboration |
| 9 | **Message Flow** between pools | No participant communication |
| 10 | **Formal Expressions** (FEEL) | Uses govaluate as pragmatic alternative |
| 11 | **Data Associations** (input/output specifications) | Implicit mapping via variables |

## 🟢 Low Priority

| # | Feature | Impact |
|---|---------|--------|
| 12 | **Escalation Event** | No escalation handling — specific use case |
| 13 | **Compensation Event** | No partial rollback — specific use case |
| 14 | **Conditional Event** | No state-change-based events — niche |
| 15 | **Link Event** | Off-page connector — non-critical |
| 16 | **Complex Gateway** | Complex merging logic — very specific, low demand |
| 17 | **Manual Task** | Emulable with UserTask |
| 18 | **Transaction** sub-process | No sub-process support, no transactions |
| 19 | **Resource Assignment** formal (resourceRole) | Simplified assignment via fields |
| 20 | **BPMN diagram rendering** to PNG (gg/fogleman) | Dependency present, metadata only |
| 21 | **gRPC** API | Planned but not implemented |

## Summary

| Priority | Count |
|----------|-------|
| 🔴 High | 1 |
| 🟡 Medium | 7 |
| 🟢 Low | 10 |
| **Total** | **18** |
